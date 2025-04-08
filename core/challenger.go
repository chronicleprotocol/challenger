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

	"github.com/defiweb/go-eth/types"
)

const slotPeriodInSec = 12

const OpPokedEventSig = "0xb9dc937c5e394d0c8f76e0e324500b88251b4c909ddc56232df10e2ea42b3c63"

type Challenger struct {
	ctx                context.Context
	address            types.Address
	provider           IScribeOptimisticProvider
	lastProcessedBlock *big.Int
	wg                 *sync.WaitGroup
}

// NewChallenger creates a new instance of Challenger.
func NewChallenger(
	ctx context.Context,
	address types.Address,
	provider IScribeOptimisticProvider,
	fromBlock int64,
	wg *sync.WaitGroup,
) *Challenger {
	var latestBlock *big.Int
	if fromBlock != 0 {
		latestBlock = big.NewInt(fromBlock)
	}
	return &Challenger{
		ctx:                ctx,
		address:            address,
		provider:           provider,
		lastProcessedBlock: latestBlock,
		wg:                 wg,
	}
}

// Gets earliest block number we can look `OpPoked` events from.
func (c *Challenger) getEarliestBlockNumber(lastBlock *big.Int, period uint16) *big.Int {
	// Calculate the earliest block number.
	blocksPerPeriod := uint64(period) / slotPeriodInSec
	if lastBlock.Cmp(big.NewInt(int64(blocksPerPeriod))) == -1 {
		return big.NewInt(0)
	}
	res := big.NewInt(0).Sub(lastBlock, big.NewInt(int64(blocksPerPeriod)))
	return res
}

func (c *Challenger) getFromBlockNumber(latestBlockNumber *big.Int, period uint16) (*big.Int, error) {
	if c.lastProcessedBlock != nil {
		return c.lastProcessedBlock, nil
	}
	if latestBlockNumber == nil {
		return nil, fmt.Errorf("latest block number is nil")
	}

	// Calculating earliest block number we can try to challenge OpPoked event from.
	earliestBlockNumber := c.getEarliestBlockNumber(latestBlockNumber, period)
	return earliestBlockNumber, nil
}

func (c *Challenger) isPokeChallengeable(poke *OpPokedEvent, challengePeriod uint16) bool {
	if poke == nil || poke.BlockNumber == nil {
		logger.
			WithField("address", c.address).
			Info("OpPoked or block number is nil")
		return false
	}
	block, err := c.provider.BlockByNumber(c.ctx, poke.BlockNumber)
	if err != nil {
		logger.
			WithField("address", c.address).
			Errorf("Failed to get block by number %d with error: %v", poke.BlockNumber, err)
		return false
	}
	challengeableSince := time.Now().Add(-time.Second * time.Duration(challengePeriod))

	// Not challengeable by time
	if block.Timestamp.Before(challengeableSince) {
		logger.
			WithField("address", c.address).
			Infof("Not challengeable by time %v", challengeableSince)
		return false
	}

	valid, err := c.provider.IsPokeSignatureValid(c.ctx, c.address, poke)
	if err != nil {
		logger.
			WithField("address", c.address).
			Errorf("Failed to verify OpPoked signature with error: %v", err)
		return false
	}
	logger.
		WithField("address", c.address).
		Infof("Is opPoke signature valid? %v", valid)

	// Only challengeable if signature is not valid
	return !valid
}

// SpawnChallenge spawns new goroutine and challenges the `OpPoked` event.
func (c *Challenger) SpawnChallenge(poke *OpPokedEvent) {
	go func() {
		logger.
			WithField("address", c.address).
			Warnf("Challenging OpPoked event from block %v", poke.BlockNumber)
		txHash, _, err := c.provider.ChallengePoke(c.ctx, c.address, poke)
		if err != nil {
			logger.
				WithField("address", c.address).
				Errorf("failed to challenge OpPoked event from block %v with error: %v", poke.BlockNumber, err)
			return
		}
		logger.
			WithField("address", c.address).
			WithField("txHash", txHash).
			Infof("Challenge successful")

		// Adding metrics
		ChallengeCounter.WithLabelValues(
			c.address.String(),
			c.provider.GetFrom(c.ctx).String(),
			txHash.String(),
		).Inc()
	}()
}

func (c *Challenger) executeTick() error {
	latestBlockNumber, err := c.provider.BlockNumber(c.ctx)
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

	logger.
		WithField("address", c.address).
		Debugf("Block number to start with: %d", fromBlockNumber)

	pokeLogs, err := c.provider.GetPokes(c.ctx, c.address, fromBlockNumber, latestBlockNumber)
	if err != nil {
		return fmt.Errorf("failed to get OpPoked events with error: %v", err)
	}

	// Set updated block we processed.
	c.lastProcessedBlock = latestBlockNumber

	// Fulfill block number in metrics
	asFloat64, _ := new(big.Float).SetInt(latestBlockNumber).Float64()
	LastScannedBlockGauge.WithLabelValues(c.address.String(), c.provider.GetFrom(c.ctx).String()).Set(asFloat64)

	if len(pokeLogs) == 0 {
		logger.
			WithField("address", c.address).
			Debugf("No logs found")
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
			logger.
				WithField("address", c.address).
				Debugf("Event from block %v is not challengeable", poke.BlockNumber)
			continue
		}

		c.SpawnChallenge(poke)
	}

	return nil
}

// Run starts the challenger processing loop.
// If you provide `subscriptionURL` - it will listen for events from WS connection otherwise, it will poll for new events every 30 seconds.
func (c *Challenger) Run() error {
	defer c.wg.Done()

	// Executing first tick
	err := c.executeTick()
	if err != nil {
		logger.
			WithField("address", c.address).
			Errorf("Failed to execute tick with error: %v", err)

		// Add error to metrics
		ErrorsCounter.WithLabelValues(
			c.address.String(),
			c.provider.GetFrom(c.ctx).String(),
			err.Error(),
		).Inc()
	}

	logger.
		WithField("address", c.address).
		Infof("Started contract monitoring")

	ticker := time.NewTicker(30 * time.Second)

	for {
		select {
		case <-c.ctx.Done():
			logger.
				WithField("address", c.address).
				Infof("Terminate challenger")
			return nil

		case t := <-ticker.C:
			logger.
				WithField("address", c.address).
				Debugf("Tick at: %v", t)

			err := c.executeTick()
			if err != nil {
				logger.
					WithField("address", c.address).
					Errorf("Failed to execute tick with error: %v", err)
				// Add error to metrics
				ErrorsCounter.WithLabelValues(
					c.address.String(),
					c.provider.GetFrom(c.ctx).String(),
					err.Error(),
				).Inc()
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
