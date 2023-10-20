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
	"github.com/defiweb/go-eth/rpc"
	"github.com/defiweb/go-eth/types"
	logger "github.com/sirupsen/logrus"
)

type ScribeOptimisticRpcProvider struct {
	client   *rpc.Client
	contract *abi.Contract
}

func NewScribeOptimisticRpcProvider(contract *abi.Contract, client *rpc.Client) *ScribeOptimisticRpcProvider {
	return &ScribeOptimisticRpcProvider{
		contract: contract,
		client:   client,
	}
}

func (s *ScribeOptimisticRpcProvider) GetChallengePeriod(ctx context.Context, address types.Address) (uint16, error) {
	opChallengePeriod := s.contract.Methods["opChallengePeriod"]
	calldata, err := opChallengePeriod.EncodeArgs()
	if err != nil {
		panic(err)
	}
	b, _, err := s.client.Call(ctx, types.Call{
		To:    &address,
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

func (s *ScribeOptimisticRpcProvider) GetPokes(
	ctx context.Context,
	address types.Address,
	fromBlock *big.Int,
	toBlock *big.Int,
) ([]*OpPokedEvent, error) {
	event := s.contract.Events["OpPoked"]

	// Fetch logs for OpPoked events.
	pokeLogs, err := s.client.GetLogs(ctx, types.FilterLogsQuery{
		Address:   []types.Address{address},
		FromBlock: types.BlockNumberFromBigIntPtr(fromBlock),
		ToBlock:   types.BlockNumberFromBigIntPtr(toBlock),
		Topics:    [][]types.Hash{{event.Topic0()}},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get OpPoked events with error: %v", err)
	}

	var result []*OpPokedEvent
	for _, poke := range pokeLogs {
		decoded, err := DecodeOpPokeEvent(event, poke)
		if err != nil {
			logger.Errorf("Failed to decode OpPoked event with error: %v", err)
			continue
		}
		result = append(result, decoded)
	}
	return result, nil
}

func (s *ScribeOptimisticRpcProvider) GetSuccessfulChallenges(
	ctx context.Context,
	address types.Address,
	fromBlock *big.Int,
	toBlock *big.Int,
) ([]*OpPokeChallengedSuccessfullyEvent, error) {
	event := s.contract.Events["OpPokeChallengedSuccessfully"]

	// Fetch logs for OpPokeChallengedSuccessfully events.
	challenges, err := s.client.GetLogs(ctx, types.FilterLogsQuery{
		Address:   []types.Address{address},
		FromBlock: types.BlockNumberFromBigIntPtr(fromBlock),
		ToBlock:   types.BlockNumberFromBigIntPtr(toBlock),
		Topics:    [][]types.Hash{{event.Topic0()}},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get OpPokeChallengedSuccessfully events with error: %v", err)
	}
	var result []*OpPokeChallengedSuccessfullyEvent
	for _, challenge := range challenges {
		decoded, err := DecodeOpPokeChallengedSuccessfullyEvent(event, challenge)
		if err != nil {
			logger.Errorf("Failed to decode OpPokeChallengedSuccessfully event with error: %v", err)
			continue
		}
		result = append(result, decoded)
	}
	return result, nil
}

func (s *ScribeOptimisticRpcProvider) constructPokeMessage(
	ctx context.Context,
	address types.Address,
	poke *OpPokedEvent,
) ([]byte, error) {
	constructMessage := s.contract.Methods["constructPokeMessage"]
	calldata, err := constructMessage.EncodeArgs(poke.PokeData)
	if err != nil {
		return nil, fmt.Errorf("failed to encode constructOpPokeMessage args: %v", err)
	}
	b, _, err := s.client.Call(ctx, types.Call{
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
		poke.PokeData.Val,
		poke.PokeData.Age,
	)
	return message, nil
}

func (s *ScribeOptimisticRpcProvider) isSchnorrSignatureAcceptable(
	ctx context.Context,
	address types.Address,
	poke *OpPokedEvent,
	message []byte,
) (bool, error) {
	isAcceptableSignature := s.contract.Methods["isAcceptableSchnorrSignatureNow"]
	calldata, err := isAcceptableSignature.EncodeArgs(message, poke.Schnorr)
	if err != nil {
		return false, fmt.Errorf("failed to encode isAcceptableSchnorrSignatureNow args: %v", err)
	}
	b, _, err := s.client.Call(ctx, types.Call{
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
		fmt.Sprintf("0x%x", poke.Schnorr.Signature),
		poke.Schnorr.Commitment,
		fmt.Sprintf("0x%x", poke.Schnorr.SignersBlob),
	)
	return res, nil
}

func (s *ScribeOptimisticRpcProvider) IsPokeSignatureValid(ctx context.Context, address types.Address, poke *OpPokedEvent) (bool, error) {
	message, err := s.constructPokeMessage(ctx, address, poke)
	if err != nil {
		return false, err
	}
	return s.isSchnorrSignatureAcceptable(ctx, address, poke, message)
}

func (s *ScribeOptimisticRpcProvider) ChallengePoke(ctx context.Context, address types.Address, poke *OpPokedEvent) (*types.Hash, *types.Transaction, error) {
	opChallenge := s.contract.Methods["opChallenge"]

	calldata, err := opChallenge.EncodeArgs(poke.Schnorr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode opChallenge args: %v", err)
	}

	// Prepare a transaction.
	tx := (&types.Transaction{}).
		SetTo(address).
		SetInput(calldata)

	return s.client.SendTransaction(ctx, *tx)
}
