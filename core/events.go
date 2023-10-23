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
	"math/big"

	"github.com/defiweb/go-eth/types"
)

type OpPokedEvent struct {
	BlockNumber *big.Int      `abi:"blockNumber"` // uint256
	Caller      types.Address `abi:"caller"`      // address
	OpFeed      types.Address `abi:"opFeed"`      // address
	Schnorr     SchnorrData   `abi:"schnorr"`     // (bytes32,address,bytes)
	PokeData    PokeData      `abi:"pokeData"`    // (uint128,uint32)
}

func (o *OpPokedEvent) Name() string {
	return "OpPoked"
}

func (o *OpPokedEvent) GetBlockNumber() *big.Int {
	return o.BlockNumber
}

type OpPokeChallengedSuccessfullyEvent struct {
	BlockNumber *big.Int      `abi:"blockNumber"` //uint256
	Challenger  types.Address `abi:"challenger"`  //address
}

func (o *OpPokeChallengedSuccessfullyEvent) Name() string {
	return "OpPokeChallengedSuccessfullyEvent"
}

func (o *OpPokeChallengedSuccessfullyEvent) GetBlockNumber() *big.Int {
	return o.BlockNumber
}
