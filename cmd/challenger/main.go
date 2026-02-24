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
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"

	challenger "github.com/chronicleprotocol/challenger/core"
	logger "github.com/sirupsen/logrus"

	"github.com/defiweb/go-eth/rpc"
	"github.com/defiweb/go-eth/rpc/transport"
	"github.com/defiweb/go-eth/txmodifier"
	"github.com/defiweb/go-eth/types"
	"github.com/defiweb/go-eth/wallet"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	FlashbotRPCURL  string
	Address         []string
	FromBlock       int64
	MaxBlockRange   uint64
	ChainID         uint64
	TransactionType string
	MetricsAddr     string
	LogLevel        string
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
		password = strings.TrimRight(string(p), "\n\r")
	}

	return wallet.NewKeyFromJSON(o.Key, password)
}

func main() {
	var opts options
	cmd := &cobra.Command{
		Use:     "run",
		Args:    cobra.NoArgs,
		Aliases: []string{"agent"},
		Run: func(cmd *cobra.Command, args []string) {
			lvl, err := logger.ParseLevel(opts.LogLevel)
			if err != nil {
				logger.Fatalf("Invalid log level %q: %v", opts.LogLevel, err)
			}
			logger.SetLevel(lvl)

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
			var ctxCancel context.CancelFunc

			if ctx == nil {
				ctx, ctxCancel = signal.NotifyContext(context.Background(), os.Interrupt)
				defer ctxCancel()
			}

			// Key generation
			key, err := opts.getKey()
			if err != nil {
				logger.Fatalf("Failed to get private key: %v", err)
			}

			// Basic TX modifiers
			txModifiers := []rpc.TXModifier{
				txmodifier.NewNonceProvider(txmodifier.NonceProviderOptions{
					UsePendingBlock: false,
					Replace:         false,
				}),
			}
			// Chain ID validation
			if opts.ChainID != 0 {
				txModifiers = append(txModifiers, txmodifier.NewChainIDProvider(txmodifier.ChainIDProviderOptions{
					ChainID: opts.ChainID,
					Replace: false,
					Cache:   true,
				}))
			}

			switch opts.TransactionType {
			case "legacy":
				txModifiers = append(txModifiers, txmodifier.NewLegacyGasFeeEstimator(txmodifier.LegacyGasFeeEstimatorOptions{
					Multiplier:  1,
					MinGasPrice: nil,
					MaxGasPrice: nil,
					Replace:     false,
				}))
			case "eip1559":
				txModifiers = append(txModifiers, txmodifier.NewEIP1559GasFeeEstimator(txmodifier.EIP1559GasFeeEstimatorOptions{
					GasPriceMultiplier:          1,
					PriorityFeePerGasMultiplier: 1,
					MinGasPrice:                 nil,
					MaxGasPrice:                 nil,
					MinPriorityFeePerGas:        nil,
					MaxPriorityFeePerGas:        nil,
					Replace:                     false,
				}))
			case "", "none":
				// Do nothing
			default:
				logger.Fatalf("Unknown transaction type: %s. Have to be legacy, eip1559 or none", opts.TransactionType)
			}

			// Create a JSON-RPC client to mainnet.
			t, err := transport.NewHTTP(transport.HTTPOptions{URL: opts.RpcURL})
			if err != nil {
				logger.Fatalf("Failed to create transport: %v", err)
			}

			// Set manual gas limit for flashbots, they might require more gas.
			//nolint:gocritic
			baseTxModifiers := append(txModifiers, txmodifier.NewGasLimitEstimator(txmodifier.GasLimitEstimatorOptions{
				MaxGas:     0,
				Multiplier: defaultGasLimitMultiplier,
			}))

			clientOptions := []rpc.ClientOptions{
				rpc.WithTransport(t),
				rpc.WithKeys(key),
				rpc.WithDefaultAddress(key.Address()),
				rpc.WithTXModifiers(baseTxModifiers...),
			}

			client, err := rpc.NewClient(clientOptions...)
			if err != nil {
				logger.Fatalf("Failed to create RPC client: %v", err)
			}

			// Create a JSON-RPC client to flashbot.
			var flashbotClient *rpc.Client
			if opts.FlashbotRPCURL != "" {
				flashbotTransport, err := transport.NewHTTP(transport.HTTPOptions{URL: opts.FlashbotRPCURL})
				if err != nil {
					logger.Fatalf("Failed to create transport: %v", err)
				}

				// Set manual gas limit for flashbots, they might require more gas.
				//nolint:gocritic
				flashbotTxModifiers := append(txModifiers, txmodifier.NewGasLimitEstimator(txmodifier.GasLimitEstimatorOptions{
					MaxGas:     challenger.MaxFlashbotGasLimit,
					Multiplier: defaultGasLimitMultiplier,
					Replace:    false,
				}))

				// TODO: tx modifiers have to be similar ?
				flashbotClientOptions := []rpc.ClientOptions{
					rpc.WithTransport(flashbotTransport),
					rpc.WithKeys(key),
					rpc.WithDefaultAddress(key.Address()),
					rpc.WithTXModifiers(flashbotTxModifiers...),
				}

				flashbotClient, err = rpc.NewClient(flashbotClientOptions...)
				if err != nil {
					logger.Fatalf("Failed to create RPC client: %v", err)
				}
			}

			// Spawning "challenger" for each address
			var wg sync.WaitGroup
			for _, address := range addresses {
				wg.Add(1)

				p := challenger.NewScribeOptimisticRPCProvider(client, flashbotClient)
				c := challenger.NewChallenger(ctx, address, p, opts.FromBlock, opts.MaxBlockRange, &wg)

				go func(addr types.Address) {
					err := c.Run()
					if err != nil {
						// Add error to metrics
						challenger.ErrorsCounter.WithLabelValues(
							addr.String(),
							p.GetFrom(ctx).String(),
						).Inc()

						logger.Fatalf("Failed to run challenger: %v", err)
					}
				}(address)
			}

			go func() {
				prometheus.MustRegister(
					challenger.ChallengeCounter,
					challenger.ErrorsCounter,
					challenger.LastScannedBlockGauge,
				)
				http.Handle("/metrics", promhttp.Handler())
				srv := &http.Server{Addr: opts.MetricsAddr} //nolint:gosec
				go func() {
					<-ctx.Done()
					if err := srv.Shutdown(context.Background()); err != nil {
						logger.WithError(err).Error("metrics server shutdown error")
					}
				}()
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					logger.WithError(err).Error("metrics server error")
				}
			}()

			wg.Wait()
		},
	}

	cmd.PersistentFlags().StringVar(&opts.SecretKey, "secret-key", "", "Private key in format `0x******` or `*******`. If provided, no need to use --keystore")
	cmd.PersistentFlags().StringVar(&opts.Key, "keystore", "", "Keystore file (NOT FOLDER), path to key .json file. If provided, no need to use --secret-key")
	cmd.PersistentFlags().StringVar(&opts.Password, "password", "", "Key raw password as text")
	cmd.PersistentFlags().StringVar(&opts.PasswordFile, "password-file", "", "Path to key password file")
	cmd.PersistentFlags().StringVar(&opts.RpcURL, "rpc-url", "", "Node HTTP RPC_URL, normally starts with https://****")
	cmd.PersistentFlags().StringVar(&opts.FlashbotRPCURL, "flashbot-rpc-url", "", "Flashbot Node HTTP RPC_URL, normally starts with https://****")
	cmd.PersistentFlags().StringArrayVarP(&opts.Address, "addresses", "a", []string{}, "ScribeOptimistic contract address. Example: `0x891E368fE81cBa2aC6F6cc4b98e684c106e2EF4f`")
	cmd.PersistentFlags().
		Int64Var(&opts.FromBlock, "from-block", 0, "Block number to start from. If not provided, binary will try to get it from given RPC")
	cmd.PersistentFlags().
		Uint64Var(&opts.MaxBlockRange, "max_block_range", 0,
			"Maximum number of blocks per eth_getLogs call (0 = unlimited)")
	cmd.PersistentFlags().Uint64Var(&opts.ChainID, "chain-id", 0, "If no chain_id provided binary will try to get chain_id from given RPC")
	cmd.PersistentFlags().StringVar(&opts.TransactionType, "tx-type", "none", "Transaction type definition, possible values are: `legacy`, `eip1559` or `none`")
	cmd.PersistentFlags().StringVar(&opts.MetricsAddr, "metrics-addr", ":9090", "Address for the Prometheus metrics server")
	cmd.PersistentFlags().StringVar(&opts.LogLevel, "log-level", "info", "Log level: trace, debug, info, warn, error, fatal, panic")

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
