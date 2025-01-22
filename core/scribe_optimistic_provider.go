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
	_ "embed"
	"errors"
	"fmt"
	"math/big"

	"github.com/defiweb/go-eth/abi"
	"github.com/defiweb/go-eth/rpc"
	"github.com/defiweb/go-eth/types"
	logger "github.com/sirupsen/logrus"
)

// MaxChallengeRetries is the maximum number of retries to send a transaction.
// NOTE: first try with flashbots, if it fails, try with mainnet client.
const MaxChallengeRetries = 6

//go:embed ScribeOptimistic.json
var scribeOptimisticContractJSON []byte

// ScribeOptimisticContractABI contains parsed contract ABI.
var ScribeOptimisticContractABI = abi.MustParseJSON(scribeOptimisticContractJSON)

// ScribeOptimisticRpcProvider implements IScribeOptimisticProvider interface and provides functionality to interact with ScribeOptimistic contract.
type ScribeOptimisticRpcProvider struct {
	client         *rpc.Client
	flashbotClient *rpc.Client
}

// NewScribeOptimisticRpcProvider creates a new instance of ScribeOptimisticRpcProvider.
// Two clients are required: one for the mainnet and one for the flashbots relay.
// Logic is simple, try to send with flashbots first, if it fails, send with the mainnet client.
func NewScribeOptimisticRpcProvider(client *rpc.Client, flashbotClient *rpc.Client) *ScribeOptimisticRpcProvider {
	return &ScribeOptimisticRpcProvider{
		client:         client,
		flashbotClient: flashbotClient,
	}
}

func (s *ScribeOptimisticRpcProvider) GetFrom(ctx context.Context) types.Address {
	accs, err := s.client.Accounts(ctx)
	if err != nil {
		logger.Errorf("failed to get accounts with error: %v", err)
		return types.ZeroAddress
	}
	if len(accs) == 0 {
		logger.Errorf("no accounts found")
		return types.ZeroAddress
	}
	return accs[0]
}

func (s *ScribeOptimisticRpcProvider) BlockByNumber(ctx context.Context, blockNumber *big.Int) (*types.Block, error) {
	return s.client.BlockByNumber(ctx, types.BlockNumberFromBigInt(blockNumber), false)
}

func (s *ScribeOptimisticRpcProvider) BlockNumber(ctx context.Context) (*big.Int, error) {
	return s.client.BlockNumber(ctx)
}

// GetChallengePeriod returns the challenge period of the contract using call.
func (s *ScribeOptimisticRpcProvider) GetChallengePeriod(ctx context.Context, address types.Address) (uint16, error) {
	opChallengePeriod := ScribeOptimisticContractABI.Methods["opChallengePeriod"]
	calldata, err := opChallengePeriod.EncodeArgs()
	if err != nil {
		panic(err)
	}
	b, _, err := s.client.Call(ctx, &types.Call{
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

// GetPokes returns list of the `OpPoked` events within the given block range under `address`.
func (s *ScribeOptimisticRpcProvider) GetPokes(
	ctx context.Context,
	address types.Address,
	fromBlock *big.Int,
	toBlock *big.Int,
) ([]*OpPokedEvent, error) {
	event := ScribeOptimisticContractABI.Events["OpPoked"]

	// Fetch logs for OpPoked events.
	pokeLogs, err := s.client.GetLogs(ctx, &types.FilterLogsQuery{
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
		decoded, err := DecodeOpPokeEvent(poke)
		if err != nil {
			logger.Errorf("Failed to decode OpPoked event with error: %v", err)
			continue
		}
		result = append(result, decoded)
	}
	return result, nil
}

// GetSuccessfulChallenges returns list of the `OpPokeChallengedSuccessfully` events within the given block range under `address`.
func (s *ScribeOptimisticRpcProvider) GetSuccessfulChallenges(
	ctx context.Context,
	address types.Address,
	fromBlock *big.Int,
	toBlock *big.Int,
) ([]*OpPokeChallengedSuccessfullyEvent, error) {
	event := ScribeOptimisticContractABI.Events["OpPokeChallengedSuccessfully"]

	// Fetch logs for OpPokeChallengedSuccessfully events.
	challenges, err := s.client.GetLogs(ctx, &types.FilterLogsQuery{
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
		decoded, err := DecodeOpPokeChallengedSuccessfullyEvent(challenge)
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
	constructMessage := ScribeOptimisticContractABI.Methods["constructPokeMessage"]
	calldata, err := constructMessage.EncodeArgs(poke.PokeData)
	if err != nil {
		return nil, fmt.Errorf("failed to encode constructOpPokeMessage args: %v", err)
	}
	b, _, err := s.client.Call(ctx, &types.Call{
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
	isAcceptableSignature := ScribeOptimisticContractABI.Methods["isAcceptableSchnorrSignatureNow"]
	calldata, err := isAcceptableSignature.EncodeArgs(message, poke.Schnorr)
	if err != nil {
		return false, fmt.Errorf("failed to encode isAcceptableSchnorrSignatureNow args: %v", err)
	}
	b, _, err := s.client.Call(ctx, &types.Call{
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

// IsPokeSignatureValid returns true if the given poke signature is valid.
// Signature validation flow described here: https://github.com/chronicleprotocol/scribe/blob/main/docs/Scribe.md#verifying-optimistic-pokes
func (s *ScribeOptimisticRpcProvider) IsPokeSignatureValid(ctx context.Context, address types.Address, poke *OpPokedEvent) (bool, error) {
	message, err := s.constructPokeMessage(ctx, address, poke)
	if err != nil {
		return false, err
	}
	return s.isSchnorrSignatureAcceptable(ctx, address, poke, message)
}

// ChallengePoke challenges the given poke by sending transaction for `opChallenge` contract function.
// Makes several attempts to send a transaction, first with flashbots, then with the mainnet client.
func (s *ScribeOptimisticRpcProvider) ChallengePoke(ctx context.Context, address types.Address, poke *OpPokedEvent) (*types.Hash, *types.Transaction, error) {
	opChallenge := ScribeOptimisticContractABI.Methods["opChallenge"]

	calldata, err := opChallenge.EncodeArgs(poke.Schnorr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode opChallenge args: %w", err)
	}

	// Prepare a transaction.
	tx := (&types.Transaction{}).
		SetTo(address).
		SetInput(calldata)

	var errs []error
	for i := 0; i < MaxChallengeRetries; i++ {
		if i <= MaxChallengeRetries/2 {
			// Try to send with flashbots first.
			hash, tx, err := s.flashbotClient.SendTransaction(ctx, tx)
			if err == nil {
				return hash, tx, nil
			}
			errs = append(errs, fmt.Errorf("try: %d failed to send tx with flashbots: %w", i, err))
		} else {
			// Try to send with the mainnet client.
			hash, tx, err := s.client.SendTransaction(ctx, tx)
			if err == nil {
				return hash, tx, nil
			}
			errs = append(errs, fmt.Errorf("try: %d failed to send tx with mainnet: %w", i, err))
		}
		i++
	}

	return nil, nil, fmt.Errorf("failed to send challenge transaction: %w", errors.Join(errs...))
}
