// Copyright (C) 2021-2023 Chronicle Labs, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/defiweb/go-eth/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockScribeOptimisticProvider struct {
	mock.Mock
}

func (s *mockScribeOptimisticProvider) BlockByNumber(ctx context.Context, blockNumber *big.Int) (*types.Block, error) {
	args := s.Called(ctx, blockNumber)
	block := args.Get(0)
	if block == nil {
		return nil, args.Error(1)
	}
	return block.(*types.Block), args.Error(1)
}

func (s *mockScribeOptimisticProvider) BlockNumber(ctx context.Context) (*big.Int, error) {
	args := s.Called(ctx)
	return args.Get(0).(*big.Int), args.Error(1)
}

func (s *mockScribeOptimisticProvider) GetChallengePeriod(ctx context.Context, address types.Address) (uint16, error) {
	args := s.Called(ctx, address)
	return uint16(args.Int(0)), args.Error(1)
}

func (s *mockScribeOptimisticProvider) GetPokes(ctx context.Context, address types.Address, fromBlock *big.Int, toBlock *big.Int) ([]*OpPokedEvent, error) {
	args := s.Called(ctx, address, fromBlock, toBlock)
	return args.Get(0).([]*OpPokedEvent), args.Error(1)
}

func (s *mockScribeOptimisticProvider) GetSuccessfulChallenges(ctx context.Context, address types.Address, fromBlock *big.Int, toBlock *big.Int) ([]*OpPokeChallengedSuccessfullyEvent, error) {
	args := s.Called(ctx, address, fromBlock, toBlock)
	return args.Get(0).([]*OpPokeChallengedSuccessfullyEvent), args.Error(1)
}

func (s *mockScribeOptimisticProvider) IsPokeSignatureValid(ctx context.Context, address types.Address, poke *OpPokedEvent) (bool, error) {
	args := s.Called(ctx, address, poke)
	return args.Bool(0), args.Error(1)
}

func (s *mockScribeOptimisticProvider) ChallengePoke(ctx context.Context, address types.Address, poke *OpPokedEvent) (*types.Hash, *types.Transaction, error) {
	args := s.Called(ctx, address, poke)
	return args.Get(0).(*types.Hash), args.Get(1).(*types.Transaction), args.Error(2)
}

func (s *mockScribeOptimisticProvider) GetFrom(ctx context.Context) types.Address {
	args := s.Called(ctx)
	return args.Get(0).(types.Address)
}

func TestGetFromBlockNumber(t *testing.T) {
	address := types.MustAddressFromHex("0x1F7acDa376eF37EC371235a094113dF9Cb4EfEe1")
	mockedProvider := new(mockScribeOptimisticProvider)

	c := NewChallenger(context.TODO(), address, mockedProvider, 0, 0, nil)
	require.NotNil(t, c)

	// Error on nil as latest block number
	b, err := c.getFromBlockNumber(nil, 600)
	assert.Error(t, err)
	assert.Nil(t, b)

	// Couldn't be less than 0
	b, err = c.getFromBlockNumber(big.NewInt(1), 600)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), b)

	b, err = c.getFromBlockNumber(big.NewInt(1000), 600)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(950), b)
}

func TestIsPokeChallengeable(t *testing.T) {
	address := types.MustAddressFromHex("0x1F7acDa376eF37EC371235a094113dF9Cb4EfEe1")
	mockedProvider := new(mockScribeOptimisticProvider)
	challengePeriod := uint16(600)
	poke := OpPokedEvent{BlockNumber: big.NewInt(1000)}

	c := NewChallenger(context.TODO(), address, mockedProvider, 0, 0, nil)
	require.NotNil(t, c)

	assert.False(t, c.isPokeChallengeable(nil, 600))
	assert.False(t, c.isPokeChallengeable(&OpPokedEvent{BlockNumber: nil}, challengePeriod))

	// False on error for getting block information
	call := mockedProvider.On("BlockByNumber", mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("error"))
	assert.False(t, c.isPokeChallengeable(&poke, challengePeriod))
	mockedProvider.AssertExpectations(t)
	call.Unset()

	// false on block older than challenge period
	ts := time.Now().Add(-time.Second * time.Duration(challengePeriod+2))
	call = mockedProvider.On("BlockByNumber", mock.Anything, mock.Anything).
		Return(&types.Block{Number: big.NewInt(1000), Timestamp: ts}, nil)
	assert.False(t, c.isPokeChallengeable(&poke, challengePeriod))
	mockedProvider.AssertExpectations(t)
	call.Unset()

	// error in signature validation also does poke unchallengeable
	// Valid signature does poke non challengeable
	call = mockedProvider.On("BlockByNumber", mock.Anything, mock.Anything).
		Return(&types.Block{Number: big.NewInt(1000), Timestamp: time.Now()}, nil)
	isPokeValidCall := mockedProvider.On("IsPokeSignatureValid", mock.Anything, mock.Anything, mock.Anything).
		Return(false, fmt.Errorf("error"))

	assert.False(t, c.isPokeChallengeable(&poke, challengePeriod))

	mockedProvider.AssertExpectations(t)
	isPokeValidCall.Unset()
	call.Unset()

	// Valid signature does poke non challengeable
	call = mockedProvider.On("BlockByNumber", mock.Anything, mock.Anything).
		Return(&types.Block{Number: big.NewInt(1000), Timestamp: time.Now()}, nil)
	isPokeValidCall = mockedProvider.On("IsPokeSignatureValid", mock.Anything, mock.Anything, mock.Anything).
		Return(true, nil)

	assert.False(t, c.isPokeChallengeable(&poke, challengePeriod))

	mockedProvider.AssertExpectations(t)
	isPokeValidCall.Unset()
	call.Unset()

	// Invalid signature does poke challengeable
	call = mockedProvider.On("BlockByNumber", mock.Anything, mock.Anything).
		Return(&types.Block{Number: big.NewInt(1000), Timestamp: time.Now()}, nil)
	isPokeValidCall = mockedProvider.On("IsPokeSignatureValid", mock.Anything, mock.Anything, mock.Anything).
		Return(false, nil)

	assert.True(t, c.isPokeChallengeable(&poke, challengePeriod))

	mockedProvider.AssertExpectations(t)
	isPokeValidCall.Unset()
	call.Unset()
}

func TestPickUnchallengedPokes(t *testing.T) {
	mkPoke := func(block int64) *OpPokedEvent {
		return &OpPokedEvent{BlockNumber: big.NewInt(block)}
	}
	mkChallenge := func(block int64) *OpPokeChallengedSuccessfullyEvent {
		return &OpPokeChallengedSuccessfullyEvent{BlockNumber: big.NewInt(block)}
	}

	t.Run("no pokes returns empty", func(t *testing.T) {
		result := PickUnchallengedPokes(nil, []*OpPokeChallengedSuccessfullyEvent{mkChallenge(100)})
		assert.Nil(t, result)
	})

	t.Run("no challenges returns all pokes", func(t *testing.T) {
		pokes := []*OpPokedEvent{mkPoke(100), mkPoke(200)}
		result := PickUnchallengedPokes(pokes, nil)
		assert.Equal(t, pokes, result)
	})

	t.Run("single poke with challenge AFTER is challenged", func(t *testing.T) {
		pokes := []*OpPokedEvent{mkPoke(100)}
		challenges := []*OpPokeChallengedSuccessfullyEvent{mkChallenge(105)}
		result := PickUnchallengedPokes(pokes, challenges)
		assert.Empty(t, result, "poke at block 100 should be filtered out because challenge at block 105 is after it")
	})

	t.Run("single poke with challenge at SAME block is challenged (couldn't happen in real life)", func(t *testing.T) {
		pokes := []*OpPokedEvent{mkPoke(100)}
		challenges := []*OpPokeChallengedSuccessfullyEvent{mkChallenge(100)}
		result := PickUnchallengedPokes(pokes, challenges)
		assert.Empty(t, result, "poke at block 100 should be filtered out because challenge is at the same block")
	})

	t.Run("single poke with challenge BEFORE is unchallenged", func(t *testing.T) {
		pokes := []*OpPokedEvent{mkPoke(100)}
		challenges := []*OpPokeChallengedSuccessfullyEvent{mkChallenge(50)}
		result := PickUnchallengedPokes(pokes, challenges)
		require.Len(t, result, 1, "poke at block 100 should remain because challenge at block 50 is for a previous poke")
		assert.Equal(t, big.NewInt(100), result[0].BlockNumber)
	})

	// Multi-poke cases (issue 1.2)

	t.Run("two pokes, first challenged between them", func(t *testing.T) {
		// sorted: [Poke@100, Challenge@105, Poke@200]
		pokes := []*OpPokedEvent{mkPoke(100), mkPoke(200)}
		challenges := []*OpPokeChallengedSuccessfullyEvent{mkChallenge(105)}
		result := PickUnchallengedPokes(pokes, challenges)
		require.Len(t, result, 1, "only poke@200 should remain unchallenged")
		assert.Equal(t, big.NewInt(200), result[0].BlockNumber)
	})

	t.Run("two pokes, second challenged after it (challenge is last element)", func(t *testing.T) {
		// sorted: [Poke@100, Poke@200, Challenge@205]
		// This is the off-by-one bug: Poke@200 is second-to-last, Challenge@205 is last
		pokes := []*OpPokedEvent{mkPoke(100), mkPoke(200)}
		challenges := []*OpPokeChallengedSuccessfullyEvent{mkChallenge(205)}
		result := PickUnchallengedPokes(pokes, challenges)
		require.Len(t, result, 1, "only poke@100 should remain, poke@200 was challenged at 205")
		assert.Equal(t, big.NewInt(100), result[0].BlockNumber)
	})

	t.Run("two pokes, both challenged", func(t *testing.T) {
		// sorted: [Poke@100, Challenge@105, Poke@200, Challenge@205]
		pokes := []*OpPokedEvent{mkPoke(100), mkPoke(200)}
		challenges := []*OpPokeChallengedSuccessfullyEvent{mkChallenge(105), mkChallenge(205)}
		result := PickUnchallengedPokes(pokes, challenges)
		assert.Empty(t, result, "both pokes were challenged")
	})

	t.Run("three pokes, middle one challenged", func(t *testing.T) {
		// sorted: [Poke@100, Poke@200, Challenge@205, Poke@300]
		pokes := []*OpPokedEvent{mkPoke(100), mkPoke(200), mkPoke(300)}
		challenges := []*OpPokeChallengedSuccessfullyEvent{mkChallenge(205)}
		result := PickUnchallengedPokes(pokes, challenges)
		require.Len(t, result, 2, "poke@100 and poke@300 should remain")
		assert.Equal(t, big.NewInt(100), result[0].BlockNumber)
		assert.Equal(t, big.NewInt(300), result[1].BlockNumber)
	})

	t.Run("two pokes, no challenges between or after", func(t *testing.T) {
		// sorted: [Challenge@50, Poke@100, Poke@200]
		pokes := []*OpPokedEvent{mkPoke(100), mkPoke(200)}
		challenges := []*OpPokeChallengedSuccessfullyEvent{mkChallenge(50)}
		result := PickUnchallengedPokes(pokes, challenges)
		require.Len(t, result, 2, "both pokes should remain, challenge@50 is for a previous poke")
		assert.Equal(t, big.NewInt(100), result[0].BlockNumber)
		assert.Equal(t, big.NewInt(200), result[1].BlockNumber)
	})
}

func TestSpawnChallengeDuplicateProtection(t *testing.T) {
	address := types.MustAddressFromHex("0x1F7acDa376eF37EC371235a094113dF9Cb4EfEe1")
	from := types.MustAddressFromHex("0x0000000000000000000000000000000000000001")
	txHash := types.MustHashFromHex("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", types.PadNone)

	t.Run("first call proceeds, second call with same block is skipped", func(t *testing.T) {
		mockedProvider := new(mockScribeOptimisticProvider)

		// Use a channel to block ChallengePoke so the goroutine stays in-flight.
		gate := make(chan struct{})
		entered := make(chan struct{})
		mockedProvider.On("ChallengePoke", mock.Anything, mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { close(entered); <-gate }).
			Return(&txHash, &types.Transaction{}, nil)
		mockedProvider.On("GetFrom", mock.Anything).Return(from)

		c := NewChallenger(context.TODO(), address, mockedProvider, 0, 0, &sync.WaitGroup{})
		poke := &OpPokedEvent{BlockNumber: big.NewInt(1000)}

		// First call should proceed and mark block 1000 as in-flight.
		c.SpawnChallenge(poke)

		// Wait for goroutine to enter ChallengePoke.
		<-entered

		// Second call with same block should be skipped (no additional goroutine).
		c.SpawnChallenge(poke)

		// ChallengePoke should have been called exactly once at this point.
		mockedProvider.AssertNumberOfCalls(t, "ChallengePoke", 1)

		// Unblock the goroutine.
		close(gate)

		// Wait for the goroutine to finish and clean up.
		require.Eventually(t, func() bool {
			c.inFlightMu.Lock()
			defer c.inFlightMu.Unlock()
			_, ok := c.inFlight[1000]
			return !ok
		}, 2*time.Second, 10*time.Millisecond)

		// After completion, block 1000 should no longer be in-flight.
		c.inFlightMu.Lock()
		_, stillInFlight := c.inFlight[1000]
		c.inFlightMu.Unlock()
		assert.False(t, stillInFlight, "block 1000 should be removed from in-flight after goroutine completes")
	})

	t.Run("block can be re-challenged after goroutine completes", func(t *testing.T) {
		mockedProvider := new(mockScribeOptimisticProvider)
		mockedProvider.On("ChallengePoke", mock.Anything, mock.Anything, mock.Anything).
			Return(&txHash, &types.Transaction{}, nil)
		mockedProvider.On("GetFrom", mock.Anything).Return(from)

		c := NewChallenger(context.TODO(), address, mockedProvider, 0, 0, &sync.WaitGroup{})
		poke := &OpPokedEvent{BlockNumber: big.NewInt(2000)}

		// First challenge.
		c.SpawnChallenge(poke)
		require.Eventually(t, func() bool {
			c.inFlightMu.Lock()
			defer c.inFlightMu.Unlock()
			_, ok := c.inFlight[2000]
			return !ok
		}, 2*time.Second, 10*time.Millisecond)

		// After first goroutine completes, block should be removed from in-flight.
		// Second call should proceed normally.
		c.SpawnChallenge(poke)
		require.Eventually(t, func() bool {
			c.inFlightMu.Lock()
			defer c.inFlightMu.Unlock()
			_, ok := c.inFlight[2000]
			return !ok
		}, 2*time.Second, 10*time.Millisecond)

		// ChallengePoke should have been called twice total.
		mockedProvider.AssertNumberOfCalls(t, "ChallengePoke", 2)
	})

	t.Run("different block numbers can be challenged concurrently", func(t *testing.T) {
		mockedProvider := new(mockScribeOptimisticProvider)

		gate := make(chan struct{})
		entered := make(chan struct{}, 2)
		mockedProvider.On("ChallengePoke", mock.Anything, mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { entered <- struct{}{}; <-gate }).
			Return(&txHash, &types.Transaction{}, nil)
		mockedProvider.On("GetFrom", mock.Anything).Return(from)

		c := NewChallenger(context.TODO(), address, mockedProvider, 0, 0, &sync.WaitGroup{})

		poke1 := &OpPokedEvent{BlockNumber: big.NewInt(3000)}
		poke2 := &OpPokedEvent{BlockNumber: big.NewInt(4000)}

		c.SpawnChallenge(poke1)
		c.SpawnChallenge(poke2)

		// Wait for both goroutines to enter ChallengePoke.
		<-entered
		<-entered

		// Both should have started since they have different block numbers.
		mockedProvider.AssertNumberOfCalls(t, "ChallengePoke", 2)

		close(gate)
		require.Eventually(t, func() bool {
			c.inFlightMu.Lock()
			defer c.inFlightMu.Unlock()
			return len(c.inFlight) == 0
		}, 2*time.Second, 10*time.Millisecond)
	})
}

func TestExecuteTick(t *testing.T) {
	address := types.MustAddressFromHex("0x1F7acDa376eF37EC371235a094113dF9Cb4EfEe1")
	from := types.MustAddressFromHex("0x0000000000000000000000000000000000000001")
	txHash := types.MustHashFromHex("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", types.PadNone)

	t.Run("error on BlockNumber failure", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		p.On("BlockNumber", mock.Anything).Return((*big.Int)(nil), fmt.Errorf("rpc down"))

		c := NewChallenger(context.TODO(), address, p, 100, 0, nil)
		err := c.executeTick()
		assert.ErrorContains(t, err, "failed to get latest block number")
		p.AssertExpectations(t)
	})

	t.Run("error on GetChallengePeriod failure", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		p.On("BlockNumber", mock.Anything).Return(big.NewInt(1000), nil)
		p.On("GetChallengePeriod", mock.Anything, address).Return(0, fmt.Errorf("contract error"))

		c := NewChallenger(context.TODO(), address, p, 100, 0, nil)
		err := c.executeTick()
		assert.ErrorContains(t, err, "failed to get challenge period")
		p.AssertExpectations(t)
	})

	t.Run("error on GetPokes failure", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		p.On("BlockNumber", mock.Anything).Return(big.NewInt(1000), nil)
		p.On("GetChallengePeriod", mock.Anything, address).Return(600, nil)
		p.On("GetPokes", mock.Anything, address, big.NewInt(100), big.NewInt(1000)).
			Return(([]*OpPokedEvent)(nil), fmt.Errorf("logs error"))

		c := NewChallenger(context.TODO(), address, p, 100, 0, nil)
		err := c.executeTick()
		assert.ErrorContains(t, err, "failed to get OpPoked events")
		p.AssertExpectations(t)
	})

	t.Run("no pokes returns nil and updates lastProcessedBlock", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		p.On("BlockNumber", mock.Anything).Return(big.NewInt(1000), nil)
		p.On("GetChallengePeriod", mock.Anything, address).Return(600, nil)
		p.On("GetPokes", mock.Anything, address, big.NewInt(100), big.NewInt(1000)).
			Return([]*OpPokedEvent{}, nil)
		p.On("GetFrom", mock.Anything).Return(from)

		c := NewChallenger(context.TODO(), address, p, 100, 0, nil)
		err := c.executeTick()
		assert.NoError(t, err)
		assert.Equal(t, big.NewInt(1000), c.lastProcessedBlock)
		p.AssertExpectations(t)
		// GetSuccessfulChallenges should not be called when there are no pokes.
		p.AssertNotCalled(t, "GetSuccessfulChallenges")
	})

	t.Run("error on GetSuccessfulChallenges failure", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		p.On("BlockNumber", mock.Anything).Return(big.NewInt(1000), nil)
		p.On("GetChallengePeriod", mock.Anything, address).Return(600, nil)
		p.On("GetPokes", mock.Anything, address, big.NewInt(100), big.NewInt(1000)).
			Return([]*OpPokedEvent{{BlockNumber: big.NewInt(500)}}, nil)
		p.On("GetFrom", mock.Anything).Return(from)
		p.On("GetSuccessfulChallenges", mock.Anything, address, big.NewInt(100), big.NewInt(1000)).
			Return(([]*OpPokeChallengedSuccessfullyEvent)(nil), fmt.Errorf("logs error"))

		c := NewChallenger(context.TODO(), address, p, 100, 0, nil)
		err := c.executeTick()
		assert.ErrorContains(t, err, "failed to get OpPokeChallengedSuccessfully events")
		p.AssertExpectations(t)
	})

	t.Run("non-challengeable poke is skipped", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		p.On("BlockNumber", mock.Anything).Return(big.NewInt(1000), nil)
		p.On("GetChallengePeriod", mock.Anything, address).Return(600, nil)
		poke := &OpPokedEvent{BlockNumber: big.NewInt(500)}
		p.On("GetPokes", mock.Anything, address, big.NewInt(100), big.NewInt(1000)).
			Return([]*OpPokedEvent{poke}, nil)
		p.On("GetFrom", mock.Anything).Return(from)
		p.On("GetSuccessfulChallenges", mock.Anything, address, big.NewInt(100), big.NewInt(1000)).
			Return([]*OpPokeChallengedSuccessfullyEvent{}, nil)
		// Block is older than challenge period — not challengeable.
		ts := time.Now().Add(-time.Second * 700)
		p.On("BlockByNumber", mock.Anything, big.NewInt(500)).
			Return(&types.Block{Number: big.NewInt(500), Timestamp: ts}, nil)

		c := NewChallenger(context.TODO(), address, p, 100, 0, nil)
		err := c.executeTick()
		assert.NoError(t, err)
		// ChallengePoke should never be called.
		p.AssertNotCalled(t, "ChallengePoke")
		p.AssertExpectations(t)
	})

	t.Run("challengeable poke triggers SpawnChallenge", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		p.On("BlockNumber", mock.Anything).Return(big.NewInt(1000), nil)
		p.On("GetChallengePeriod", mock.Anything, address).Return(600, nil)
		poke := &OpPokedEvent{BlockNumber: big.NewInt(500)}
		p.On("GetPokes", mock.Anything, address, big.NewInt(100), big.NewInt(1000)).
			Return([]*OpPokedEvent{poke}, nil)
		// First GetFrom call is synchronous (executeTick metrics).
		p.On("GetFrom", mock.Anything).Return(from).Once()
		p.On("GetSuccessfulChallenges", mock.Anything, address, big.NewInt(100), big.NewInt(1000)).
			Return([]*OpPokeChallengedSuccessfullyEvent{}, nil)
		// Block is recent — within challenge period.
		p.On("BlockByNumber", mock.Anything, big.NewInt(500)).
			Return(&types.Block{Number: big.NewInt(500), Timestamp: time.Now()}, nil)
		// Signature is invalid — challengeable.
		p.On("IsPokeSignatureValid", mock.Anything, address, poke).Return(false, nil)
		p.On("ChallengePoke", mock.Anything, address, poke).
			Return(&txHash, &types.Transaction{}, nil)
		// Second GetFrom call is async (SpawnChallenge goroutine metrics).
		goroutineDone := make(chan struct{})
		p.On("GetFrom", mock.Anything).
			Run(func(args mock.Arguments) { close(goroutineDone) }).
			Return(from).Once()

		c := NewChallenger(context.TODO(), address, p, 100, 0, &sync.WaitGroup{})
		err := c.executeTick()
		assert.NoError(t, err)

		// Wait for the SpawnChallenge goroutine to complete.
		<-goroutineDone

		p.AssertCalled(t, "ChallengePoke", mock.Anything, address, poke)
		p.AssertExpectations(t)
	})

	t.Run("already challenged poke is filtered out", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		p.On("BlockNumber", mock.Anything).Return(big.NewInt(1000), nil)
		p.On("GetChallengePeriod", mock.Anything, address).Return(600, nil)
		poke := &OpPokedEvent{BlockNumber: big.NewInt(500)}
		p.On("GetPokes", mock.Anything, address, big.NewInt(100), big.NewInt(1000)).
			Return([]*OpPokedEvent{poke}, nil)
		p.On("GetFrom", mock.Anything).Return(from)
		// Challenge exists after the poke — poke is filtered out.
		p.On("GetSuccessfulChallenges", mock.Anything, address, big.NewInt(100), big.NewInt(1000)).
			Return([]*OpPokeChallengedSuccessfullyEvent{{BlockNumber: big.NewInt(505)}}, nil)

		c := NewChallenger(context.TODO(), address, p, 100, 0, nil)
		err := c.executeTick()
		assert.NoError(t, err)
		// No pokes remain after filtering, so no block lookups or challenges.
		p.AssertNotCalled(t, "BlockByNumber")
		p.AssertNotCalled(t, "ChallengePoke")
		p.AssertExpectations(t)
	})

	t.Run("lastProcessedBlock is used as fromBlock on second tick", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		// First tick: fromBlock=100, latestBlock=1000.
		p.On("BlockNumber", mock.Anything).Return(big.NewInt(1000), nil).Once()
		p.On("GetChallengePeriod", mock.Anything, address).Return(600, nil)
		p.On("GetPokes", mock.Anything, address, big.NewInt(100), big.NewInt(1000)).
			Return([]*OpPokedEvent{}, nil).Once()
		p.On("GetFrom", mock.Anything).Return(from)

		c := NewChallenger(context.TODO(), address, p, 100, 0, nil)
		err := c.executeTick()
		assert.NoError(t, err)
		assert.Equal(t, big.NewInt(1000), c.lastProcessedBlock)

		// Second tick: fromBlock should now be 1000 (lastProcessedBlock), latestBlock=2000.
		p.On("BlockNumber", mock.Anything).Return(big.NewInt(2000), nil).Once()
		p.On("GetPokes", mock.Anything, address, big.NewInt(1000), big.NewInt(2000)).
			Return([]*OpPokedEvent{}, nil).Once()

		err = c.executeTick()
		assert.NoError(t, err)
		assert.Equal(t, big.NewInt(2000), c.lastProcessedBlock)
		p.AssertExpectations(t)
	})
}

func TestRun(t *testing.T) {
	address := types.MustAddressFromHex("0x1F7acDa376eF37EC371235a094113dF9Cb4EfEe1")
	from := types.MustAddressFromHex("0x0000000000000000000000000000000000000001")

	t.Run("context cancellation exits cleanly and calls wg.Done", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		// executeTick will run once on startup — provide happy path with no pokes.
		p.On("BlockNumber", mock.Anything).Return(big.NewInt(1000), nil)
		p.On("GetChallengePeriod", mock.Anything, address).Return(600, nil)
		p.On("GetPokes", mock.Anything, address, big.NewInt(100), big.NewInt(1000)).
			Return([]*OpPokedEvent{}, nil)
		p.On("GetFrom", mock.Anything).Return(from)

		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		wg.Add(1)

		c := NewChallenger(ctx, address, p, 100, 0, &wg)

		done := make(chan struct{})
		go func() {
			err := c.Run()
			assert.NoError(t, err)
			close(done)
		}()

		// Cancel context to stop the loop.
		cancel()

		// wg.Wait should return because Run calls wg.Done.
		wg.Wait()
		<-done
	})

	t.Run("tick error does not stop the loop", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		// First tick (startup): error.
		p.On("BlockNumber", mock.Anything).Return((*big.Int)(nil), fmt.Errorf("rpc down"))
		// GetFrom is called by handleTickError before the loop starts.
		tickHandled := make(chan struct{})
		p.On("GetFrom", mock.Anything).
			Run(func(args mock.Arguments) { close(tickHandled) }).
			Return(from)

		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		wg.Add(1)

		c := NewChallenger(ctx, address, p, 100, 0, &wg)

		done := make(chan struct{})
		go func() {
			err := c.Run()
			assert.NoError(t, err)
			close(done)
		}()

		// Wait for handleTickError to complete (signals the loop is starting).
		<-tickHandled
		// Even though tick errored, Run should still be running.
		// Cancel to exit cleanly.
		cancel()
		wg.Wait()
		<-done
	})
}

func TestGetEarliestBlockNumber(t *testing.T) {
	address := types.MustAddressFromHex("0x1F7acDa376eF37EC371235a094113dF9Cb4EfEe1")
	c := NewChallenger(context.TODO(), address, nil, 0, 0, nil)

	t.Run("block less than blocksPerPeriod returns zero", func(t *testing.T) {
		// period=600, blocksPerPeriod = 600/12 = 50, lastBlock=30 < 50
		result := c.getEarliestBlockNumber(big.NewInt(30), 600)
		assert.Equal(t, big.NewInt(0), result)
	})

	t.Run("block equal to blocksPerPeriod returns zero", func(t *testing.T) {
		// period=600, blocksPerPeriod = 50, lastBlock=50 is not less than 50
		result := c.getEarliestBlockNumber(big.NewInt(50), 600)
		assert.Equal(t, 0, result.Cmp(big.NewInt(0)))
	})

	t.Run("block greater than blocksPerPeriod returns difference", func(t *testing.T) {
		// period=600, blocksPerPeriod = 50, lastBlock=1000 -> 1000-50 = 950
		result := c.getEarliestBlockNumber(big.NewInt(1000), 600)
		assert.Equal(t, big.NewInt(950), result)
	})

	t.Run("small period", func(t *testing.T) {
		// period=12, blocksPerPeriod = 12/12 = 1, lastBlock=100 -> 99
		result := c.getEarliestBlockNumber(big.NewInt(100), 12)
		assert.Equal(t, big.NewInt(99), result)
	})
}

func TestSpawnChallengeErrorPath(t *testing.T) {
	address := types.MustAddressFromHex("0x1F7acDa376eF37EC371235a094113dF9Cb4EfEe1")

	t.Run("nil poke returns early", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		c := NewChallenger(context.TODO(), address, p, 0, 0, &sync.WaitGroup{})

		c.SpawnChallenge(nil)

		// No RPC calls should be made.
		p.AssertNotCalled(t, "ChallengePoke")
	})

	t.Run("nil BlockNumber returns early", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		c := NewChallenger(context.TODO(), address, p, 0, 0, &sync.WaitGroup{})

		c.SpawnChallenge(&OpPokedEvent{BlockNumber: nil})

		// No RPC calls should be made.
		p.AssertNotCalled(t, "ChallengePoke")
	})

	t.Run("ChallengePoke error does not record metrics", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		called := make(chan struct{})
		p.On("ChallengePoke", mock.Anything, mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { close(called) }).
			Return((*types.Hash)(nil), (*types.Transaction)(nil), fmt.Errorf("tx failed"))

		c := NewChallenger(context.TODO(), address, p, 0, 0, &sync.WaitGroup{})
		poke := &OpPokedEvent{BlockNumber: big.NewInt(5000)}

		c.SpawnChallenge(poke)
		<-called

		// Wait for goroutine to finish (clean up inFlight after ChallengePoke returns).
		require.Eventually(t, func() bool {
			c.inFlightMu.Lock()
			defer c.inFlightMu.Unlock()
			_, ok := c.inFlight[5000]
			return !ok
		}, 2*time.Second, 10*time.Millisecond)

		// ChallengePoke was called but GetFrom should NOT be called (metrics not recorded on error).
		p.AssertCalled(t, "ChallengePoke", mock.Anything, mock.Anything, mock.Anything)
		p.AssertNotCalled(t, "GetFrom")

		// In-flight entry should be cleaned up.
		c.inFlightMu.Lock()
		_, stillInFlight := c.inFlight[5000]
		c.inFlightMu.Unlock()
		assert.False(t, stillInFlight)
	})
}

func TestGetPokesInRange(t *testing.T) {
	address := types.MustAddressFromHex("0x1F7acDa376eF37EC371235a094113dF9Cb4EfEe1")

	t.Run("maxBlockRange=0 makes single call without chunking", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		poke := &OpPokedEvent{BlockNumber: big.NewInt(500)}
		p.On("GetPokes", mock.Anything, address, big.NewInt(100), big.NewInt(1000)).
			Return([]*OpPokedEvent{poke}, nil).Once()

		c := NewChallenger(context.TODO(), address, p, 0, 0, nil)
		result, err := c.getPokesInRange(big.NewInt(100), big.NewInt(1000))
		assert.NoError(t, err)
		assert.Equal(t, []*OpPokedEvent{poke}, result)
		p.AssertExpectations(t)
		p.AssertNumberOfCalls(t, "GetPokes", 1)
	})

	t.Run("range fits within one chunk makes single call", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		poke := &OpPokedEvent{BlockNumber: big.NewInt(150)}
		p.On("GetPokes", mock.Anything, address, big.NewInt(100), big.NewInt(200)).
			Return([]*OpPokedEvent{poke}, nil).Once()

		c := NewChallenger(context.TODO(), address, p, 0, 500, nil)
		result, err := c.getPokesInRange(big.NewInt(100), big.NewInt(200))
		assert.NoError(t, err)
		assert.Equal(t, []*OpPokedEvent{poke}, result)
		p.AssertExpectations(t)
		p.AssertNumberOfCalls(t, "GetPokes", 1)
	})

	t.Run("range splits into exact chunks", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		poke1 := &OpPokedEvent{BlockNumber: big.NewInt(200)}
		poke2 := &OpPokedEvent{BlockNumber: big.NewInt(700)}
		// chunk1: [100, 599], chunk2: [600, 1099] — but toBlock is 1099 so exact fit
		p.On("GetPokes", mock.Anything, address, big.NewInt(100), big.NewInt(599)).
			Return([]*OpPokedEvent{poke1}, nil).Once()
		p.On("GetPokes", mock.Anything, address, big.NewInt(600), big.NewInt(1099)).
			Return([]*OpPokedEvent{poke2}, nil).Once()

		c := NewChallenger(context.TODO(), address, p, 0, 500, nil)
		result, err := c.getPokesInRange(big.NewInt(100), big.NewInt(1099))
		assert.NoError(t, err)
		assert.Equal(t, []*OpPokedEvent{poke1, poke2}, result)
		p.AssertExpectations(t)
		p.AssertNumberOfCalls(t, "GetPokes", 2)
	})

	t.Run("last chunk is truncated to toBlock", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		poke1 := &OpPokedEvent{BlockNumber: big.NewInt(200)}
		poke2 := &OpPokedEvent{BlockNumber: big.NewInt(650)}
		// chunk1: [100, 599], chunk2: [600, 700] (truncated from 1099)
		p.On("GetPokes", mock.Anything, address, big.NewInt(100), big.NewInt(599)).
			Return([]*OpPokedEvent{poke1}, nil).Once()
		p.On("GetPokes", mock.Anything, address, big.NewInt(600), big.NewInt(700)).
			Return([]*OpPokedEvent{poke2}, nil).Once()

		c := NewChallenger(context.TODO(), address, p, 0, 500, nil)
		result, err := c.getPokesInRange(big.NewInt(100), big.NewInt(700))
		assert.NoError(t, err)
		assert.Equal(t, []*OpPokedEvent{poke1, poke2}, result)
		p.AssertExpectations(t)
		p.AssertNumberOfCalls(t, "GetPokes", 2)
	})

	t.Run("error on first chunk stops iteration", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		p.On("GetPokes", mock.Anything, address, big.NewInt(100), big.NewInt(599)).
			Return(([]*OpPokedEvent)(nil), fmt.Errorf("rpc error")).Once()

		c := NewChallenger(context.TODO(), address, p, 0, 500, nil)
		result, err := c.getPokesInRange(big.NewInt(100), big.NewInt(1099))
		assert.ErrorContains(t, err, "rpc error")
		assert.Nil(t, result)
		p.AssertExpectations(t)
		p.AssertNumberOfCalls(t, "GetPokes", 1)
	})

	t.Run("fromBlock == toBlock makes exactly one call", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		poke := &OpPokedEvent{BlockNumber: big.NewInt(500)}
		p.On("GetPokes", mock.Anything, address, big.NewInt(500), big.NewInt(500)).
			Return([]*OpPokedEvent{poke}, nil).Once()

		c := NewChallenger(context.TODO(), address, p, 0, 500, nil)
		result, err := c.getPokesInRange(big.NewInt(500), big.NewInt(500))
		assert.NoError(t, err)
		assert.Equal(t, []*OpPokedEvent{poke}, result)
		p.AssertExpectations(t)
		p.AssertNumberOfCalls(t, "GetPokes", 1)
	})
}

func TestGetChallengesInRange(t *testing.T) {
	address := types.MustAddressFromHex("0x1F7acDa376eF37EC371235a094113dF9Cb4EfEe1")

	t.Run("maxBlockRange=0 makes single call without chunking", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		ch := &OpPokeChallengedSuccessfullyEvent{BlockNumber: big.NewInt(500)}
		p.On("GetSuccessfulChallenges", mock.Anything, address, big.NewInt(100), big.NewInt(1000)).
			Return([]*OpPokeChallengedSuccessfullyEvent{ch}, nil).Once()

		c := NewChallenger(context.TODO(), address, p, 0, 0, nil)
		result, err := c.getChallengesInRange(big.NewInt(100), big.NewInt(1000))
		assert.NoError(t, err)
		assert.Equal(t, []*OpPokeChallengedSuccessfullyEvent{ch}, result)
		p.AssertExpectations(t)
		p.AssertNumberOfCalls(t, "GetSuccessfulChallenges", 1)
	})

	t.Run("range fits within one chunk makes single call", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		ch := &OpPokeChallengedSuccessfullyEvent{BlockNumber: big.NewInt(150)}
		p.On("GetSuccessfulChallenges", mock.Anything, address, big.NewInt(100), big.NewInt(200)).
			Return([]*OpPokeChallengedSuccessfullyEvent{ch}, nil).Once()

		c := NewChallenger(context.TODO(), address, p, 0, 500, nil)
		result, err := c.getChallengesInRange(big.NewInt(100), big.NewInt(200))
		assert.NoError(t, err)
		assert.Equal(t, []*OpPokeChallengedSuccessfullyEvent{ch}, result)
		p.AssertExpectations(t)
		p.AssertNumberOfCalls(t, "GetSuccessfulChallenges", 1)
	})

	t.Run("range splits into exact chunks", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		ch1 := &OpPokeChallengedSuccessfullyEvent{BlockNumber: big.NewInt(200)}
		ch2 := &OpPokeChallengedSuccessfullyEvent{BlockNumber: big.NewInt(700)}
		p.On("GetSuccessfulChallenges", mock.Anything, address, big.NewInt(100), big.NewInt(599)).
			Return([]*OpPokeChallengedSuccessfullyEvent{ch1}, nil).Once()
		p.On("GetSuccessfulChallenges", mock.Anything, address, big.NewInt(600), big.NewInt(1099)).
			Return([]*OpPokeChallengedSuccessfullyEvent{ch2}, nil).Once()

		c := NewChallenger(context.TODO(), address, p, 0, 500, nil)
		result, err := c.getChallengesInRange(big.NewInt(100), big.NewInt(1099))
		assert.NoError(t, err)
		assert.Equal(t, []*OpPokeChallengedSuccessfullyEvent{ch1, ch2}, result)
		p.AssertExpectations(t)
		p.AssertNumberOfCalls(t, "GetSuccessfulChallenges", 2)
	})

	t.Run("last chunk is truncated to toBlock", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		ch1 := &OpPokeChallengedSuccessfullyEvent{BlockNumber: big.NewInt(200)}
		ch2 := &OpPokeChallengedSuccessfullyEvent{BlockNumber: big.NewInt(650)}
		p.On("GetSuccessfulChallenges", mock.Anything, address, big.NewInt(100), big.NewInt(599)).
			Return([]*OpPokeChallengedSuccessfullyEvent{ch1}, nil).Once()
		p.On("GetSuccessfulChallenges", mock.Anything, address, big.NewInt(600), big.NewInt(700)).
			Return([]*OpPokeChallengedSuccessfullyEvent{ch2}, nil).Once()

		c := NewChallenger(context.TODO(), address, p, 0, 500, nil)
		result, err := c.getChallengesInRange(big.NewInt(100), big.NewInt(700))
		assert.NoError(t, err)
		assert.Equal(t, []*OpPokeChallengedSuccessfullyEvent{ch1, ch2}, result)
		p.AssertExpectations(t)
		p.AssertNumberOfCalls(t, "GetSuccessfulChallenges", 2)
	})

	t.Run("error on first chunk stops iteration", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		p.On("GetSuccessfulChallenges", mock.Anything, address, big.NewInt(100), big.NewInt(599)).
			Return(([]*OpPokeChallengedSuccessfullyEvent)(nil), fmt.Errorf("rpc error")).Once()

		c := NewChallenger(context.TODO(), address, p, 0, 500, nil)
		result, err := c.getChallengesInRange(big.NewInt(100), big.NewInt(1099))
		assert.ErrorContains(t, err, "rpc error")
		assert.Nil(t, result)
		p.AssertExpectations(t)
		p.AssertNumberOfCalls(t, "GetSuccessfulChallenges", 1)
	})

	t.Run("fromBlock == toBlock makes exactly one call", func(t *testing.T) {
		p := new(mockScribeOptimisticProvider)
		ch := &OpPokeChallengedSuccessfullyEvent{BlockNumber: big.NewInt(500)}
		p.On("GetSuccessfulChallenges", mock.Anything, address, big.NewInt(500), big.NewInt(500)).
			Return([]*OpPokeChallengedSuccessfullyEvent{ch}, nil).Once()

		c := NewChallenger(context.TODO(), address, p, 0, 500, nil)
		result, err := c.getChallengesInRange(big.NewInt(500), big.NewInt(500))
		assert.NoError(t, err)
		assert.Equal(t, []*OpPokeChallengedSuccessfullyEvent{ch}, result)
		p.AssertExpectations(t)
		p.AssertNumberOfCalls(t, "GetSuccessfulChallenges", 1)
	})
}
