# Challenger GoLang version

Challenger searches for `opPoked` events for `ScribeOptimistic` contract. Verifies poke schnorr signature and challenges it, if it's invalid.

```bash
Usage:
  run [flags]

Aliases:
  run, agent

Flags:
  -a, --addresses 0x891E368fE81cBa2aC6F6cc4b98e684c106e2EF4f   ScribeOptimistic contract address. Example: 0x891E368fE81cBa2aC6F6cc4b98e684c106e2EF4f
      --chain-id uint                                          If no chain_id provided binary will try to get chain_id from given RPC
      --from-block uint                                        Block number to start from. If not provided, binary will try to get it from given RPC
  -h, --help                                                   help for run
      --keystore string                                        Keystore file (NOT FOLDER), path to key .json file. If provided, no need to use --secret-key
      --password string                                        Key raw password as text
      --password-file string                                   Path to key password file
      --rpc-url string                                         Node HTTP RPC_URL, normally starts with https://****
      --secret-key 0x******                                    Private key in format 0x****** or `*******`. If provided, no need to use --keystore
```


## Example

Starting with private key

```bash
challenger run --addresses 0x891E368fE81cBa2aC6F6cc4b98e684c106e2EF4f --rpc-url http://localhost:3334 --secret-key 0x******
```

Starting with key file and password

```bash
challenger run -a 0x891E368fE81cBa2aC6F6cc4b98e684c106e2EF4f --rpc-url http://localhost:3334 --keystore /path/to/key.json --password-file /path/to/file
```

## Logging level

By default `challenger` uses log level `info`.
If you want to get debug information use `RUST_LOG=debug` env variable !


## Building docker image

SERVER_VERSION have to be same as release but without `v`, if release is `v0.0.10` then `SERVER_VERSION=0.0.10`

```bash
docker build -t challenger-go .
```

usage:

```bash
docker run --rm challenger-go

Usage:
  run [flags]

Aliases:
  run, agent

Flags:
  -a, --addresses 0x891E368fE81cBa2aC6F6cc4b98e684c106e2EF4f   ScribeOptimistic contract address. Example: 0x891E368fE81cBa2aC6F6cc4b98e684c106e2EF4f
      --chain-id uint                                          If no chain_id provided binary will try to get chain_id from given RPC
      --from-block uint                                        Block number to start from. If not provided, binary will try to get it from given RPC
  -h, --help                                                   help for run
      --keystore string                                        Keystore file (NOT FOLDER), path to key .json file. If provided, no need to use --secret-key
      --password string                                        Key raw password as text
      --password-file string                                   Path to key password file
      --rpc-url string                                         Node HTTP RPC_URL, normally starts with https://****
      --secret-key 0x******                                    Private key in format 0x****** or `*******`. If provided, no need to use --keystore
```

```bash
docker run --it --rm --name challenger-go run -a ADDRESS --rpc-url http://localhost:3334 --secret-key asdfasdfas
```