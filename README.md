# Challenger GoLang version

Challenger searches for `opPoked` events for `ScribeOptimistic` contract. It verifies the poked Schnorr signature and challenges if it's invalid.

```bash
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
      --subscription-url string                                [Optional] Used if you want to subscribe to events rather than poll, typically starts with wss://****
```

Note that in *all* cases you must provide `--rpc-url`, but if you want to use event driven listening instead of polling you also need to provide `--subscription-url`.

## Example

Starting with private key

```bash
challenger run --addresses 0x891E368fE81cBa2aC6F6cc4b98e684c106e2EF4f --rpc-url http://localhost:3334 --secret-key 0x******
```

Starting with key file and password

```bash
challenger run -a 0x891E368fE81cBa2aC6F6cc4b98e684c106e2EF4f --rpc-url http://localhost:3334 --keystore /path/to/key.json --password-file /path/to/file
```

## Building docker image

SERVER_VERSION have to be same as release but without `v`, if release is `v0.0.10` then `SERVER_VERSION=0.0.10`

```bash
docker build -t challenger-go .
```

Usage:

```bash
docker run --rm challenger-go
```

Full example:

```bash
docker run --it --rm --name challenger-go run -a ADDRESS --rpc-url http://localhost:3334 --secret-key asdfasdfas
```