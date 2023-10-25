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

package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"sync"

	challenger "github.com/chronicleprotocol/challenger/core"
	"github.com/defiweb/go-eth/wallet"
	logger "github.com/sirupsen/logrus"

	"github.com/defiweb/go-eth/rpc"
	"github.com/defiweb/go-eth/rpc/transport"
	"github.com/defiweb/go-eth/txmodifier"
	"github.com/defiweb/go-eth/types"
	"github.com/spf13/cobra"
)

const (
	defaultGasLimitMultiplier = 1.25
)

type options struct {
	SecretKey       string
	Key             string
	Password        string
	PasswordFile    string
	RpcURL          string
	SubscriptionURL string
	Address         []string
	FromBlock       uint64
	ChainID         uint64
}

// Checks and return private key based on given options
func (o *options) getKey() (*wallet.PrivateKey, error) {
	if o.SecretKey != "" {
		return wallet.NewKeyFromBytes(types.MustBytesFromHex(o.SecretKey)), nil
	}

	if o.Key == "" {
		return nil, fmt.Errorf("please provide key using `--keystore` flag")
	}

	stat, err := os.Stat(o.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to open keystore file: %v", err)
	}
	if stat.IsDir() {
		return nil, fmt.Errorf("keystore file is a directory")
	}

	if o.Password == "" && o.PasswordFile == "" {
		return nil, fmt.Errorf("please provide password using `--password` or `--password-file` flag")
	}
	var password string
	if o.Password != "" {
		password = o.Password
	} else if o.PasswordFile != "" {
		p, err := os.ReadFile(o.PasswordFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read password file: %v", err)
		}
		password = string(p)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read password file: %v", err)
	}
	return wallet.NewKeyFromJSON(o.Key, password)
}

func main() {
	var opts options
	cmd := &cobra.Command{
		Use:     "run",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"agent"},
		Run: func(cmd *cobra.Command, args []string) {
			// TODO: update after completion
			logger.SetLevel(logger.DebugLevel)

			logger.Debugf("Hello, Challenger!")

			if opts.RpcURL == "" {
				logger.Errorf("Please provide Rpc URL using `--rpc-url` flag")
				return
			}

			// Parsing list of addresses
			if len(opts.Address) == 0 {
				logger.Errorf("Please provide address using `--addresses` flag")
				return
			}
			var addresses []types.Address
			for _, address := range opts.Address {
				a, err := types.AddressFromHex(address)
				if err != nil {
					logger.Errorf("Failed to parse given address %s with error: %v", address, err)
					return
				}
				addresses = append(addresses, a)
			}

			// Building context
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			t, err := transport.NewHTTP(transport.HTTPOptions{URL: opts.RpcURL})
			if err != nil {
				logger.Fatalf("Failed to create transport: %v", err)
			}

			// Key generation
			key, err := opts.getKey()
			if err != nil {
				logger.Fatalf("Failed to get private key: %v", err)
			}

			clientOptions := []rpc.ClientOptions{
				rpc.WithTransport(t),
				rpc.WithKeys(key),
				rpc.WithDefaultAddress(key.Address()),
				rpc.WithTXModifiers(
					txmodifier.NewNonceProvider(false),
					txmodifier.NewGasLimitEstimator(defaultGasLimitMultiplier, 0, 0),
					txmodifier.NewLegacyGasFeeEstimator(1, nil, nil),
				),
			}

			if opts.ChainID != 0 {
				clientOptions = append(clientOptions, rpc.WithChainID(opts.ChainID))
			}

			// Create a JSON-RPC client.
			client, err := rpc.NewClient(clientOptions...)
			if err != nil {
				logger.Fatalf("Failed to create RPC client: %v", err)
			}

			var wg sync.WaitGroup
			for _, address := range addresses {
				wg.Add(1)

				p := challenger.NewScribeOptimisticRpcProvider(client)
				c := challenger.NewChallenger(ctx, address, p, 0, opts.SubscriptionURL, &wg)

				go func() {
					err := c.Run()
					if err != nil {
						logger.Fatalf("Failed to run challenger: %v", err)
					}
				}()
			}

			wg.Wait()
		},
	}

	cmd.PersistentFlags().StringVar(&opts.SecretKey, "secret-key", "", "Private key in format `0x******` or `*******`. If provided, no need to use --keystore")
	cmd.PersistentFlags().StringVar(&opts.Key, "keystore", "", "Keystore file (NOT FOLDER), path to key .json file. If provided, no need to use --secret-key")
	cmd.PersistentFlags().StringVar(&opts.Password, "password", "", "Key raw password as text")
	cmd.PersistentFlags().StringVar(&opts.PasswordFile, "password-file", "", "Path to key password file")
	cmd.PersistentFlags().StringVar(&opts.RpcURL, "rpc-url", "", "Node HTTP RPC_URL, normally starts with https://****")
	cmd.PersistentFlags().StringVar(&opts.SubscriptionURL, "subscription-url", "", "[Optional] Used if you want to subscribe to events rather than poll, typically starts with wss://****")
	cmd.PersistentFlags().StringArrayVarP(&opts.Address, "addresses", "a", []string{}, "ScribeOptimistic contract address. Example: `0x891E368fE81cBa2aC6F6cc4b98e684c106e2EF4f`")
	cmd.PersistentFlags().Uint64Var(&opts.FromBlock, "from-block", 0, "Block number to start from. If not provided, binary will try to get it from given RPC")
	cmd.PersistentFlags().Uint64Var(&opts.ChainID, "chain-id", 0, "If no chain_id provided binary will try to get chain_id from given RPC")

	_ = cmd.Execute()
}
