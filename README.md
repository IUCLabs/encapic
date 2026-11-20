# encapic

Dependency-less Go CLI client for [encapi](https://github.com/IUCLabs/encapi),
used as the Puppet exec External Node Classifier (ENC) on the Kubernetes
compilers.

It replaces the previous uv/python ENC script, whose first-invocation
dependency resolution failed on fresh compiler pods (exit 135/2), causing agent
catalog failures. encapic is a single static binary that depends on the Go
standard library only.

## Usage

```
encapic <certname>
```

encapic fetches the cobbler-wire ENC document from

```
${ENCAPI_URL}/cblr/svc/op/puppet/hostname/<certname>
```

reshapes it, and prints the resulting ENC YAML to stdout. It exits non-zero on
any HTTP or parse failure — including a 404 for an unknown node — so the puppet
exec `node_terminus` fails safe rather than compiling an empty catalog.

- The HTTP request has a 10s timeout.

`encapic --help` lists every command.

### Configuration

The encapi base URL comes from the first of:

1. the `ENCAPI_URL` environment variable;
2. `${XDG_CONFIG_HOME:-~/.config}/encapi/encapic.conf`;
3. `/etc/encapi/encapic.conf`;
4. the compiled-in default `http://encapi.encapi.svc.cluster.local`.

Conf files are shell-sourceable `KEY=VALUE` (`ENCAPI_URL=https://...`).
`encapic init` prompts for the URL (default
`https://encapi.example.com`), writes the user conf and offers to
install shell completion. With no config found, an interactive run prints a hint to
stderr; non-TTY runs (the puppet exec ENC) stay silent.

### Shell completion

```
encapic completion bash|zsh            # print the script
encapic completion bash|zsh --install  # install under ${XDG_DATA_HOME:-~/.local/share}
```

The RPM also installs both scripts system-wide.

### Pinning a host to `testing`

```
encapic set-testing <certname>
```

Sets the node's environment to `testing` in encapi (requires the encapi
oidc-auth release). A `testing` node's ENC omits `environment`, so any
`puppet agent --environment X` is honoured; the host stays in `testing` until
the next terragrunt-enc apply resets it.

It mints a JWT from `${VAULT_ADDR}/v1/identity/oidc/token/${ENCAPI_OIDC_ROLE}`
(defaults `https://vault.example.com:8200`, `encapi-testing`) using
`VAULT_TOKEN` or `~/.vault-token`, trusting `VAULT_CACERT` when set, then
`PUT`s `{"environment":"testing"}` to
`${ENCAPI_URL}/api/v1/nodes/<certname>/environment`. `ENCAPI_URL` and
`VAULT_ADDR` must be `https` and redirects are never followed, so neither
the Vault token nor the JWT is sent over plain http or to another host.

### Restoring a host from `testing`

```
encapic unset-testing <certname>
```

Restores the environment the node had before `set-testing` and prints it
(requires the encapi testing-restore endpoint). It authenticates like
`set-testing` and sends `DELETE ${ENCAPI_URL}/api/v1/nodes/<certname>/environment/testing`;
a node that is not in `testing` fails with encapi's error message.

## Behaviour (drop-in for the python ENC)

encapic reproduces the previous python script byte-for-byte:

- `classes` (a cobbler-wire map keyed by role, or a list) becomes a list of
  role names;
- `parameters.enc_role` is set to that same list;
- when `environment` is present, `parameters.enc_env` is set to it, and the
  top-level `environment` key is dropped when it equals `testing`;
- output keys are alphabetically sorted, matching python's `yaml.dump`.

Example output:

```yaml
classes:
- roles::infra::storage::vault
environment: develop
parameters:
  enc_env: develop
  enc_role:
  - roles::infra::storage::vault
```

## Design: why the cobbler endpoint + hand-emitted YAML

encapi also exposes `/api/v1/nodes/<certname>/enc`, which serves the fully
reshaped document. encapic deliberately consumes the **cobbler-wire** endpoint
(`/cblr/svc/op/puppet/hostname/<certname>`) and reshapes it locally so its
output matches the python script it replaces byte-for-byte — meaning the swap
changes nothing the puppet agent sees. The consumed YAML has a small, fixed
shape and is hand-parsed; the emitted YAML is hand-written. This keeps encapic
on the standard library only, which is the entire point of the rewrite.

## Development

```
make build   # static binary into dist/
make test    # go test -v -race ./...
make lint    # golangci-lint
make fmt     # gofmt -w .
make rpm     # binary + RPM into dist/ via goreleaser, publishing nothing
```

Release: `make minor` (etc.) tags `vX.Y.Z` and pushes it; the tag mirrors to
GitHub, where the `release` workflow runs goreleaser, which attaches
`encapic_linux_amd64` and the RPM (each with a signed `.sha256`) to the GitHub
release.

## Installation

The kubernetes compilers fetch `encapic_linux_amd64` from the Gitea release in
an initContainer and install it to `/opt/bin/encapic`.

The VM puppet masters install the RPM from `rpm-internal`:

```
dnf install encapic
```

It ships:

- `/usr/bin/encapic` — the binary;
- `/usr/bin/encapic-enc` — the ENC entrypoint. Puppet's exec `node_terminus`
  invokes `external_nodes` with a certname and no environment, so the shim
  sources the puppet-managed `/etc/encapi/encapic.conf` before exec'ing the
  binary. Point `external_nodes` at this, not at `/usr/bin/encapic`;
- bash and zsh completion scripts.
