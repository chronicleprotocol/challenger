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

	c := NewChallenger(context.TODO(), address, mockedProvider, 0, nil)
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

	c := NewChallenger(context.TODO(), address, mockedProvider, 0, nil)
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
		mockedProvider.On("ChallengePoke", mock.Anything, mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { <-gate }).
			Return(&txHash, &types.Transaction{}, nil)
		mockedProvider.On("GetFrom", mock.Anything).Return(from)

		c := NewChallenger(context.TODO(), address, mockedProvider, 0, &sync.WaitGroup{})
		poke := &OpPokedEvent{BlockNumber: big.NewInt(1000)}

		// First call should proceed and mark block 1000 as in-flight.
		c.SpawnChallenge(poke)

		// Give goroutine time to start and hit the gate.
		time.Sleep(50 * time.Millisecond)

		// Second call with same block should be skipped (no additional goroutine).
		c.SpawnChallenge(poke)

		// ChallengePoke should have been called exactly once at this point.
		mockedProvider.AssertNumberOfCalls(t, "ChallengePoke", 1)

		// Unblock the goroutine.
		close(gate)

		// Wait for the goroutine to finish and clean up.
		time.Sleep(50 * time.Millisecond)

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

		c := NewChallenger(context.TODO(), address, mockedProvider, 0, &sync.WaitGroup{})
		poke := &OpPokedEvent{BlockNumber: big.NewInt(2000)}

		// First challenge.
		c.SpawnChallenge(poke)
		time.Sleep(50 * time.Millisecond)

		// After first goroutine completes, block should be removed from in-flight.
		// Second call should proceed normally.
		c.SpawnChallenge(poke)
		time.Sleep(50 * time.Millisecond)

		// ChallengePoke should have been called twice total.
		mockedProvider.AssertNumberOfCalls(t, "ChallengePoke", 2)
	})

	t.Run("different block numbers can be challenged concurrently", func(t *testing.T) {
		mockedProvider := new(mockScribeOptimisticProvider)

		gate := make(chan struct{})
		mockedProvider.On("ChallengePoke", mock.Anything, mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { <-gate }).
			Return(&txHash, &types.Transaction{}, nil)
		mockedProvider.On("GetFrom", mock.Anything).Return(from)

		c := NewChallenger(context.TODO(), address, mockedProvider, 0, &sync.WaitGroup{})

		poke1 := &OpPokedEvent{BlockNumber: big.NewInt(3000)}
		poke2 := &OpPokedEvent{BlockNumber: big.NewInt(4000)}

		c.SpawnChallenge(poke1)
		c.SpawnChallenge(poke2)

		time.Sleep(50 * time.Millisecond)

		// Both should have started since they have different block numbers.
		mockedProvider.AssertNumberOfCalls(t, "ChallengePoke", 2)

		close(gate)
		time.Sleep(50 * time.Millisecond)
	})
}
