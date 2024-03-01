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

	c := NewChallenger(context.TODO(), address, mockedProvider, 0, "", nil)
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

	c := NewChallenger(context.TODO(), address, mockedProvider, 0, "", nil)
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
