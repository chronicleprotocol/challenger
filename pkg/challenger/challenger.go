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

package challenger

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
)

const SLOT_PERIOD_IN_SEC = 12

type PokeData struct {
	Val uint64 `abi:"val"` // uint128
	Age uint32 `abi:"age"` // uint32
}

type SchnorrData struct {
	Signature   [32]byte      `abi:"signature"`   // bytes32
	Commitment  types.Address `abi:"commitment"`  // address
	SignersBlob []byte        `abi:"signersBlob"` // bytes
}

type Challenger struct {
	ctx                context.Context
	address            types.Address
	fromBlock          uint64
	client             *rpc.Client
	contract           *abi.Contract
	lastProcessedBlock *big.Int
	wg                 *sync.WaitGroup
}

func NewChallenger(
	ctx context.Context,
	address types.Address,
	contract *abi.Contract,
	fromBlock uint64,
	client *rpc.Client,
	wg *sync.WaitGroup,
) *Challenger {
	return &Challenger{
		ctx:       ctx,
		address:   address,
		fromBlock: fromBlock,
		client:    client,
		contract:  contract,
		wg:        wg,
	}
}

func (c *Challenger) getChallengePeriod() (uint16, error) {
	opChallengePeriod := c.contract.Methods["opChallengePeriod"]
	calldata, err := opChallengePeriod.EncodeArgs()
	if err != nil {
		panic(err)
	}
	b, _, err := c.client.Call(c.ctx, types.Call{
		To:    &c.address,
		Input: calldata,
	}, types.LatestBlockNumber)

	if err != nil {
		return 0, fmt.Errorf("failed to call opChallengePeriod with error: %v", err)
	}

	// Decode the result.
	var period uint16
	err = opChallengePeriod.DecodeValues(b, &period)
	if err != nil {
		return 0, fmt.Errorf("failed to decode opChallengePeriod result with error: %v", err)
	}
	return period, nil
}

// Gets earliest block number we can look `OpPoked` events from.
func (c *Challenger) getEarliestBlockNumber(lastBlock *big.Int, period uint16) (*big.Int, error) {
	// Calculate earliest block number.
	blocksPerPeriod := uint64(period) / SLOT_PERIOD_IN_SEC
	res := big.NewInt(0).Sub(lastBlock, big.NewInt(int64(blocksPerPeriod)))
	return res, nil
}

func (c *Challenger) getOpPokes(fromBlock *big.Int) ([]*OpPokedEvent, error) {
	event := c.contract.Events["OpPoked"]

	// Fetch logs for OpPoked events.
	pokeLogs, err := c.client.GetLogs(c.ctx, types.FilterLogsQuery{
		Address:   []types.Address{c.address},
		FromBlock: types.BlockNumberFromBigIntPtr(fromBlock),
		Topics:    [][]types.Hash{{event.Topic0()}},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get OpPoked events with error: %v", err)
	}

	var result []*OpPokedEvent
	for _, poke := range pokeLogs {
		decoded, err := c.decodeOpPoke(poke)
		if err != nil {
			logger.Errorf("Failed to decode OpPoked event with error: %v", err)
			continue
		}
		result = append(result, decoded)
	}
	return result, nil
}

func (c *Challenger) getSuccessfulChallenges(fromBlock *big.Int) ([]*OpPokeChallengedSuccessfullyEvent, error) {
	event := c.contract.Events["OpPokeChallengedSuccessfully"]

	// Fetch logs for OpPokeChallengedSuccessfully events.
	challenges, err := c.client.GetLogs(c.ctx, types.FilterLogsQuery{
		Address:   []types.Address{c.address},
		FromBlock: types.BlockNumberFromBigIntPtr(fromBlock),
		Topics:    [][]types.Hash{{event.Topic0()}},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get OpPokeChallengedSuccessfully events with error: %v", err)
	}
	var result []*OpPokeChallengedSuccessfullyEvent
	for _, challenge := range challenges {
		decoded, err := c.decodeOpPokeChallengedSuccessfully(challenge)
		if err != nil {
			logger.Errorf("Failed to decode OpPokeChallengedSuccessfully event with error: %v", err)
			continue
		}
		result = append(result, decoded)
	}
	return result, nil
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

func (c *Challenger) decodeOpPoke(log types.Log) (*OpPokedEvent, error) {
	event := c.contract.Events["OpPoked"]

	var schnorrData SchnorrData
	var pokeData PokeData
	var caller, opFeed types.Address
	// OpPoked(address,address,(bytes32,address,bytes),(uint128,uint32))
	err := event.DecodeValues(log.Topics, log.Data, &caller, &opFeed, &schnorrData, &pokeData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode event data with error: %v\n", err)
	}
	return &OpPokedEvent{
		BlockNumber: log.BlockNumber,
		Caller:      caller,
		OpFeed:      opFeed,
		Schnorr:     schnorrData,
		PokeData:    pokeData,
	}, nil
}

func (c *Challenger) decodeOpPokeChallengedSuccessfully(log types.Log) (*OpPokeChallengedSuccessfullyEvent, error) {
	event := c.contract.Events["OpPokeChallengedSuccessfully"]

	var challenger types.Address
	var b []byte
	err := event.DecodeValues(log.Topics, log.Data, &challenger, &b)
	if err != nil {
		return nil, fmt.Errorf("failed to decode event data with error: %v\n", err)
	}
	return &OpPokeChallengedSuccessfullyEvent{
		BlockNumber: log.BlockNumber,
		Challenger:  challenger,
	}, nil
}

func (c *Challenger) isPokeChallengeable(poke *OpPokedEvent, challengePeriod uint16) bool {
	if poke == nil || poke.BlockNumber == nil {
		return false
	}
	block, err := c.client.BlockByNumber(c.ctx, types.BlockNumberFromBigInt(poke.BlockNumber), false)
	if err != nil {
		logger.Errorf("Failed to get block by number %d with error: %v", poke.BlockNumber, err)
		return false
	}
	challengeableFrom := time.Now().Add(-time.Second * time.Duration(challengePeriod))

	// Not challengeable by time
	if block.Timestamp.Before(challengeableFrom) {
		return false
	}

	valid, err := poke.IsSignatureValid(c.ctx, c.client, c.contract, c.address)
	if err != nil {
		logger.Errorf("Failed to check if signature is valid with error: %v", err)
		return false
	}

	// Challengeable if signature is not valid
	return !valid
}

func (c *Challenger) challengeOpPokedEvent(event *OpPokedEvent) error {
	opChallenge := c.contract.Methods["opChallenge"]

	calldata, err := opChallenge.EncodeArgs(event.Schnorr)
	if err != nil {
		return fmt.Errorf("failed to encode opChallenge args: %v", err)
	}

	// Prepare a transaction.
	tx := (&types.Transaction{}).
		SetTo(c.address).
		SetInput(calldata)

	txHash, _, err := c.client.SendTransaction(c.ctx, *tx)
	if err != nil {
		return fmt.Errorf("failed to send opChallenge transaction with error: %v", err)
	}

	// Print the transaction hash.
	logger.Debugf("opChallenge Transaction hash: %s", txHash.String())
	return nil
}

func (c *Challenger) executeTick() error {
	latestBlockNumber, err := c.client.BlockNumber(c.ctx)
	if err != nil {
		return fmt.Errorf("failed to get latest block number with error: %v", err)
	}

	// Fetching challenge period.
	period, err := c.getChallengePeriod()
	if err != nil {
		return fmt.Errorf("failed to get challenge period with error: %v", err)
	}

	fromBlockNumber, err := c.getFromBlockNumber(latestBlockNumber, period)
	logger.Debugf("Block number to start with: %d", fromBlockNumber)

	pokeLogs, err := c.getOpPokes(fromBlockNumber)
	if err != nil {
		return fmt.Errorf("failed to get OpPoked events with error: %v", err)
	}

	// Set updated block we processed.
	c.lastProcessedBlock = latestBlockNumber

	if len(pokeLogs) == 0 {
		logger.Debugf("No logs found")
		return nil
	}

	challenges, err := c.getSuccessfulChallenges(fromBlockNumber)
	if err != nil {
		return fmt.Errorf("failed to get OpPokeChallengedSuccessfully events with error: %v", err)
	}
	// Filtering out pokes that were already challenged.
	pokes := pickUnchallengedPokes(pokeLogs, challenges)

	for _, poke := range pokes {
		if !c.isPokeChallengeable(poke, period) {
			logger.Debugf("Event from block %v is not challengeable", poke.BlockNumber)
			continue
		}
		logger.Infof("Challenging OpPoked event from block %v", poke.BlockNumber)
		err = c.challengeOpPokedEvent(poke)
		if err != nil {
			return fmt.Errorf("failed to challenge OpPoked event from block %v with error: %v", poke.BlockNumber, err)
		}
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
}

// Checks if `OpPoked` event has `OpPokeChallengedSuccessfully` event after it and before next `OpPoked` event.
// If it does, then we don't need to challenge it.
func pickUnchallengedPokes(pokes []*OpPokedEvent, challenges []*OpPokeChallengedSuccessfullyEvent) []*OpPokedEvent {
	if len(challenges) == 0 {
		return pokes
	}
	if len(pokes) == 0 {
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
	fmt.Println(sortable)
	for i, event := range sortable {
		switch event.(type) {
		case *OpPokedEvent:
			if i == len(sortable)-1 {
				result = append(result, event.(*OpPokedEvent))
				continue
			}
			if len(sortable)-1 > i+1 && sortable[i+1].Name() == "OpPokeChallengedSuccessfullyEvent" {
				continue
			}
			result = append(result, event.(*OpPokedEvent))
		case *OpPokeChallengedSuccessfullyEvent:
			continue
		}
	}

	return result
}
