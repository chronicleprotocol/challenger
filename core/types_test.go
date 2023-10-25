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
	"math/big"
	"testing"

	"github.com/defiweb/go-eth/types"
	"github.com/stretchr/testify/require"
)

func TestDecodeOpPokeChallengedSuccessfullyEvent(t *testing.T) {
	blockNumber := big.NewInt(123)
	log := types.Log{
		Topics: []types.Hash{
			types.MustHashFromHex("0xac50cef58b3aef7f7c30349f5e4a342a29d2325a02eafc8dacfdba391e6d5db3", types.PadNone),
			types.MustHashFromHex("0x0000000000000000000000001f7acda376ef37ec371235a094113df9cb4efee1", types.PadNone),
		},
		Data:        types.MustBytesFromHex("0x00000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000004bd2a556b00000000000000000000000000000000000000000000000000000000"),
		BlockNumber: blockNumber,
	}

	event, err := DecodeOpPokeChallengedSuccessfullyEvent(log)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.Equal(t, blockNumber, event.BlockNumber)
	require.Equal(t, types.MustAddressFromHex("0x1F7acDa376eF37EC371235a094113dF9Cb4EfEe1"), event.Challenger)
}

func TestDecodeOpPokeEvent(t *testing.T) {
	blockNumber := big.NewInt(123)
	log := types.Log{
		BlockNumber: blockNumber,
		Topics: []types.Hash{
			types.MustHashFromHex("0xb9dc937c5e394d0c8f76e0e324500b88251b4c909ddc56232df10e2ea42b3c63", types.PadNone),
			types.MustHashFromHex("0x0000000000000000000000001f7acda376ef37ec371235a094113df9cb4efee1", types.PadNone),
			types.MustHashFromHex("0x0000000000000000000000006813eb9362372eef6200f3b1dbc3f819671cba69", types.PadNone),
		},
	}

	event, err := DecodeOpPokeEvent(log)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.Equal(t, blockNumber, event.BlockNumber)
	require.Equal(t, types.MustAddressFromHex("0x1F7acDa376eF37EC371235a094113dF9Cb4EfEe1"), event.Caller)
	require.Equal(t, types.MustAddressFromHex("0x6813eb9362372eef6200f3b1dbc3f819671cba69"), event.OpFeed)
}
