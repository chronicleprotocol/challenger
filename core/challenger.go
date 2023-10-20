//  Copyright (C) 2021-2023 Chronicle Labs, Inc.
//
//  This program is free software: you can redistribute it and/or modify
//  it under the terms of the GNU Affero General Public License as
//  published by the Free Software Foundation, either version 3 of the
//  License, or (at your option) any later version.
//
//  This program is distributed in the hope that it will be useful,
//  but WITHOUT ANY WARRANTY; without even the implied warranty of
//  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//  GNU Affero General Public License for more details.
//
//  You should have received a copy of the GNU Affero General Public License
//  along with this program.  If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/defiweb/go-eth/abi"
	"github.com/defiweb/go-eth/rpc"
	"github.com/defiweb/go-eth/types"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

const SlotPeriodInSec = 12

const OpPokedEventSig = "0xb9dc937c5e394d0c8f76e0e324500b88251b4c909ddc56232df10e2ea42b3c63"

type Challenger struct {
	ctx                context.Context
	address            types.Address
	fromBlock          uint64
	subscriptionURL    string
	provider           IScribeOptimisticProvider
	client             *rpc.Client
	contract           *abi.Contract
	lastProcessedBlock *big.Int
	wg                 *sync.WaitGroup
}

func NewChallenger(
	ctx context.Context,
	address types.Address,
	provider IScribeOptimisticProvider,
	contract *abi.Contract,
	fromBlock uint64,
	subscriptionURL string,
	client *rpc.Client,
	wg *sync.WaitGroup,
) *Challenger {
	return &Challenger{
		ctx:             ctx,
		address:         address,
		provider:        provider,
		fromBlock:       fromBlock,
		client:          client,
		contract:        contract,
		wg:              wg,
		subscriptionURL: subscriptionURL,
	}
}

// Gets earliest block number we can look `OpPoked` events from.
func (c *Challenger) getEarliestBlockNumber(lastBlock *big.Int, period uint16) (*big.Int, error) {
	// Calculate earliest block number.
	blocksPerPeriod := uint64(period) / SlotPeriodInSec
	res := big.NewInt(0).Sub(lastBlock, big.NewInt(int64(blocksPerPeriod)))
	return res, nil
}

func (c *Challenger) getFromBlockNumber(latestBlockNumber *big.Int, period uint16) (*big.Int, error) {
	if c.lastProcessedBlock != nil {
		return c.lastProcessedBlock, nil
	}
	if latestBlockNumber == nil {
		return nil, fmt.Errorf("latest block number is nil")
	}

	// Calculating earliest block number we can try to challenge OpPoked event from.
	earliestBlockNumber, err := c.getEarliestBlockNumber(latestBlockNumber, period)
	if err != nil {
		return nil, fmt.Errorf("failed to get earliest block number with error: %v", err)
	}
	return earliestBlockNumber, nil
}

func (c *Challenger) isPokeChallengeable(poke *OpPokedEvent, challengePeriod uint16) bool {
	if poke == nil || poke.BlockNumber == nil {
		logger.Info("OpPoked or block number is nil")
		return false
	}
	block, err := c.client.BlockByNumber(c.ctx, types.BlockNumberFromBigInt(poke.BlockNumber), false)
	if err != nil {
		logger.Errorf("Failed to get block by number %d with error: %v", poke.BlockNumber, err)
		return false
	}
	challengeableSince := time.Now().Add(-time.Second * time.Duration(challengePeriod))

	// Not challengeable by time
	if block.Timestamp.Before(challengeableSince) {
		logger.Infof("Not challengeable by time %v", challengeableSince)
		return false
	}

	valid, err := c.provider.IsPokeSignatureValid(c.ctx, c.address, poke)
	if err != nil {
		logger.Errorf("Failed to verify OpPoked signature with error: %v", err)
		return false
	}
	logger.Infof("Is opPoke signature valid? %v", valid)

	// Only challengeable if signature is not valid
	return !valid
}

func (c *Challenger) executeTick() error {
	latestBlockNumber, err := c.client.BlockNumber(c.ctx)
	if err != nil {
		return fmt.Errorf("failed to get latest block number with error: %v", err)
	}

	// Fetching challenge period.
	period, err := c.provider.GetChallengePeriod(c.ctx, c.address)
	if err != nil {
		return fmt.Errorf("failed to get challenge period with error: %v", err)
	}

	fromBlockNumber, err := c.getFromBlockNumber(latestBlockNumber, period)
	if err != nil {
		return fmt.Errorf("failed to get blocknumber from period: %v", err)
	}

	logger.Debugf("[%v] Block number to start with: %d", c.address, fromBlockNumber)

	pokeLogs, err := c.provider.GetPokes(c.ctx, c.address, fromBlockNumber, latestBlockNumber)
	if err != nil {
		return fmt.Errorf("failed to get OpPoked events with error: %v", err)
	}

	// Set updated block we processed.
	c.lastProcessedBlock = latestBlockNumber

	if len(pokeLogs) == 0 {
		logger.Debugf("No logs found")
		return nil
	}

	challenges, err := c.provider.GetSuccessfulChallenges(c.ctx, c.address, fromBlockNumber, latestBlockNumber)
	if err != nil {
		return fmt.Errorf("failed to get OpPokeChallengedSuccessfully events with error: %v", err)
	}
	// Filtering out pokes that were already challenged.
	pokes := PickUnchallengedPokes(pokeLogs, challenges)

	for _, poke := range pokes {
		if !c.isPokeChallengeable(poke, period) {
			logger.Debugf("Event from block %v is not challengeable", poke.BlockNumber)
			continue
		}
		logger.Warnf("Challenging OpPoked event from block %v", poke.BlockNumber)
		txHash, _, err := c.provider.ChallengePoke(c.ctx, c.address, poke)
		if err != nil {
			return fmt.Errorf("failed to challenge OpPoked event from block %v with error: %v", poke.BlockNumber, err)
		}
		logger.Infof("Challenge transaction hash: %v", txHash.String())
	}

	return nil
}

func (c *Challenger) Run() error {
	defer c.wg.Done()

	// Executing first tick
	err := c.executeTick()
	if err != nil {
		logger.Errorf("Failed to execute tick with error: %v", err)
	}

	logger.Infof("Monitoring contract %v", c.address)

	if c.subscriptionURL == "" { // We poll
		ticker := time.NewTicker(30 * time.Second)

		for {
			select {
			case <-c.ctx.Done():
				logger.Infof("Terminate challenger")
				return nil

			case t := <-ticker.C:
				logger.Debugf("Tick at: %v", t)

				err := c.executeTick()
				if err != nil {
					logger.Errorf("Failed to execute tick with error: %v", err)
				}
			}
		}
	} else { // Event based
		return c.Listen()
	}
}

func (c *Challenger) Listen() error {
	logger.Infof("Listening for events from %v", c.address)
	ethcli, err := ethclient.Dial(c.subscriptionURL)
	if err != nil {
		return err
	}

	query := ethereum.FilterQuery{
		Addresses: []common.Address{common.HexToAddress(c.address.String())},
	}

	logs := make(chan gethtypes.Log)

	sub, err := ethcli.SubscribeFilterLogs(c.ctx, query, logs)
	if err != nil {
		return err
	}

	opPokedEvent := c.contract.Events["OpPoked"]

	for {
		select {
		case <-c.ctx.Done():
			logger.Infof("Terminate challenger")
			return nil
		case err := <-sub.Err():
			return err
		case evlog := <-logs:
			if evlog.Topics[0].Hex() != OpPokedEventSig {
				logger.Infof("Event occurred, but is not 'opPoked': %s", evlog.Topics[0].Hex())
				continue
			}

			// Marshal go-ethereum data to defiweb types
			addr, err := types.AddressFromBytes(evlog.Address.Bytes())
			if err != nil {
				return err
			}
			logger.Infof("opPoked event for %v", addr)

			topics := make([]types.Hash, 0)
			for _, topic := range evlog.Topics {
				t, err := types.HashFromBytes(topic.Bytes(), types.PadLeft)
				if err != nil {
					return err
				}
				topics = append(topics, t)
			}
			log := types.Log{
				Address:     addr,
				Topics:      topics,
				Data:        evlog.Data,
				BlockNumber: new(big.Int).SetUint64(evlog.BlockNumber),
			}

			poke, err := DecodeOpPokeEvent(opPokedEvent, log)
			if err != nil {
				return err
			}

			period, err := c.provider.GetChallengePeriod(c.ctx, c.address)
			if err != nil {
				return fmt.Errorf("failed to get challenge period with error: %v", err)
			}

			if c.isPokeChallengeable(poke, period) {
				logger.Warnf("Challenging opPoke sent from %v", common.BytesToAddress(evlog.Topics[1].Bytes()))
				txHash, _, err := c.provider.ChallengePoke(c.ctx, c.address, poke)
				if err != nil {
					return fmt.Errorf(
						"failed to challenge OpPoked event from block %v with error: %v", poke.BlockNumber, err)
				}
				logger.Infof("Challenge transaction hash: %v", txHash.String())
			} else {
				logger.Debugf("Event from block %v is not challengeable", poke.BlockNumber)
			}
		}
	}
}

// PickUnchallengedPokes Checks if `OpPoked` event has `OpPokeChallengedSuccessfully` event after it and before next `OpPoked` event.
// If it does, then we don't need to challenge it.
func PickUnchallengedPokes(pokes []*OpPokedEvent, challenges []*OpPokeChallengedSuccessfullyEvent) []*OpPokedEvent {
	if len(pokes) == 0 || len(challenges) == 0 {
		return pokes
	}

	var result []*OpPokedEvent

	if len(pokes) == 1 {
		for _, challenge := range challenges {
			if challenge.BlockNumber.Cmp(pokes[0].BlockNumber) == -1 {
				return result
			}
		}
		return pokes
	}
	sortable := make([]SortableEvent, len(pokes)+len(challenges))
	for i, poke := range pokes {
		sortable[i] = poke
	}
	for i, challenge := range challenges {
		sortable[i+len(pokes)] = challenge
	}
	sort.Slice(sortable, func(i, j int) bool {
		return sortable[i].GetBlockNumber().Cmp(sortable[j].GetBlockNumber()) == -1
	})
	for i, event := range sortable {
		switch ev := event.(type) {
		case *OpPokedEvent:
			if i == len(sortable)-1 {
				result = append(result, ev)
				continue
			}
			if len(sortable)-1 > i+1 && sortable[i+1].Name() == "OpPokeChallengedSuccessfullyEvent" {
				continue
			}
			result = append(result, ev)
		case *OpPokeChallengedSuccessfullyEvent:
			continue
		}
	}

	return result
}
