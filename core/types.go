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

	"github.com/defiweb/go-eth/abi"
	"github.com/defiweb/go-eth/types"
)

type PokeData struct {
	Val *big.Int `abi:"val"` // uint128
	Age uint32   `abi:"age"` // uint32
}

type SchnorrData struct {
	Signature   [32]byte      `abi:"signature"`   // bytes32
	Commitment  types.Address `abi:"commitment"`  // address
	SignersBlob []byte        `abi:"signersBlob"` // bytes
}

type SortableEvent interface {
	// Name returns the name of the event.
	Name() string
	// GetBlockNumber returns the block number of the event.
	GetBlockNumber() *big.Int
}

// IScribeOptimisticProvider is the interface for the ScribeOptimistic contract with required functions for challenger.
type IScribeOptimisticProvider interface {
	// OpPokedEvent returns the `OpPoked` event from the contract ABI.
	OpPokedEvent() *abi.Event

	// OpPokeChallengedSuccessfullyEvent returns the `OpPokeChallengedSuccessfully` event from the contract ABI.
	OpPokeChallengedSuccessfullyEvent() *abi.Event

	// GetChallengePeriod returns the challenge period of the contract.
	GetChallengePeriod(ctx context.Context, address types.Address) (uint16, error)

	// GetPokes returns the `OpPoked` events within the given block range.
	GetPokes(ctx context.Context, address types.Address, fromBlock *big.Int, toBlock *big.Int) ([]*OpPokedEvent, error)

	// GetSuccessfulChallenges returns the `OpPokeChallengedSuccessfully` events within the given block range.
	GetSuccessfulChallenges(ctx context.Context, address types.Address, fromBlock *big.Int, toBlock *big.Int) ([]*OpPokeChallengedSuccessfullyEvent, error)

	// IsPokeSignatureValid returns true if the given poke signature is valid.
	IsPokeSignatureValid(ctx context.Context, address types.Address, poke *OpPokedEvent) (bool, error)

	// ChallengePoke challenges the given poke.
	ChallengePoke(ctx context.Context, address types.Address, poke *OpPokedEvent) (*types.Hash, *types.Transaction, error)
}

// DecodeOpPokeEvent Decodes the OpPoked event from the given log.
// NOTE: 1st argument must be `OpPoked` event from contract ABI. (`contract.Events["OpPoked"]`)
func DecodeOpPokeEvent(event *abi.Event, log types.Log) (*OpPokedEvent, error) {
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

// DecodeOpPokeChallengedSuccessfullyEvent Decodes the OpPokeChallengedSuccessfully event from the given log.
// NOTE: 1st argument must be `OpPokeChallengedSuccessfully` event from contract ABI. (`contract.Events["OpPokeChallengedSuccessfully"]`)
func DecodeOpPokeChallengedSuccessfullyEvent(event *abi.Event, log types.Log) (*OpPokeChallengedSuccessfullyEvent, error) {
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
