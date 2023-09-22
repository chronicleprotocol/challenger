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

	"github.com/defiweb/go-eth/abi"
	"github.com/defiweb/go-eth/rpc"
	"github.com/defiweb/go-eth/types"
	logger "github.com/sirupsen/logrus"
)

type SortableEvent interface {
	// Name returns the name of the event.
	Name() string
	// GetBlockNumber returns the block number of the event.
	GetBlockNumber() *big.Int
}

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

func (o *OpPokedEvent) constructMessage(
	ctx context.Context,
	client *rpc.Client,
	contract *abi.Contract,
	address types.Address,
) ([]byte, error) {
	constructMessage := contract.Methods["constructPokeMessage"]
	calldata, err := constructMessage.EncodeArgs(o.PokeData)
	if err != nil {
		return nil, fmt.Errorf("failed to encode constructOpPokeMessage args: %v", err)
	}
	b, _, err := client.Call(ctx, types.Call{
		To:    &address,
		Input: calldata,
	}, types.LatestBlockNumber)

	if err != nil {
		return nil, fmt.Errorf("failed to call constructOpPokeMessage with error: %v", err)
	}

	// Decode the result.
	var message []byte
	err = constructMessage.DecodeValues(b, &message)
	if err != nil {
		return nil, fmt.Errorf("failed to decode constructOpPokeMessage result with error: %v", err)
	}
	logger.Debugf(
		"cast call %v 'constructPokeMessage((uint128,uint32))' '(%v,%v)'",
		address,
		o.PokeData.Val,
		o.PokeData.Age,
	)
	return message, nil
}

func (o *OpPokedEvent) checkIsAcceptableShnorrSignature(
	ctx context.Context,
	client *rpc.Client,
	contract *abi.Contract,
	address types.Address,
	message []byte,
) (bool, error) {
	isAcceptableSignature := contract.Methods["isAcceptableSchnorrSignatureNow"]
	calldata, err := isAcceptableSignature.EncodeArgs(message, o.Schnorr)
	if err != nil {
		return false, fmt.Errorf("failed to encode isAcceptableSchnorrSignatureNow args: %v", err)
	}
	b, _, err := client.Call(ctx, types.Call{
		To:    &address,
		Input: calldata,
	}, types.LatestBlockNumber)

	if err != nil {
		return false, fmt.Errorf("failed to call isAcceptableSchnorrSignatureNow with error: %v", err)
	}

	// Decode the result.
	var res bool
	err = isAcceptableSignature.DecodeValues(b, &res)
	if err != nil {
		return false, fmt.Errorf("failed to decode isAcceptableSchnorrSignatureNow result with error: %v", err)
	}
	logger.Debugf(
		"cast call %v 'isAcceptableSchnorrSignatureNow(bytes32,(bytes32,address,bytes))(bool)' %s '(%s,%v,%s)'",
		address,
		fmt.Sprintf("0x%x", message),
		fmt.Sprintf("0x%x", o.Schnorr.Signature),
		o.Schnorr.Commitment,
		fmt.Sprintf("0x%x", o.Schnorr.SignersBlob),
	)
	return res, nil
}

// IsSignatureValid checks if the shnorr signature is valid for poke data.
func (o *OpPokedEvent) IsSignatureValid(ctx context.Context,
	client *rpc.Client,
	contract *abi.Contract,
	address types.Address,
) (bool, error) {
	message, err := o.constructMessage(ctx, client, contract, address)
	if err != nil {
		return false, err
	}
	return o.checkIsAcceptableShnorrSignature(ctx, client, contract, address, message)
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
