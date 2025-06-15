# Pkv - PAID Key Value storage

Pkv is a [PAID](https://github.com/atticplaygroup/prex/wiki/paid-service#paid-services) version of a key-value storage. It functions as a Redis client, utilizing the Redis protocol to interact with the underlying storage. The goal is for it to operate effectively in various environments with the support of [Prex exchanges](https://github.com/atticplaygroup/prex/wiki/paid-service#paid-services), which issue quota tokens to ensure that all resource usages, such as bandwidth, storage space, and time, are appropriately compensated for each request to store or retrieve data.

## Installation

```bash
go install github.com/atticplaygroup/pkv/cmd/pkv@latest
```

## Usage

### Setup the environment variables

```bash
cp .env.example .env
```

Remember to change secrets like JWT_SECRET.

### Run Pkv service

```bash
go run cmd/pkv/pkv.go
```

### Run tests for pkv

```bash
bash scripts/run-tests.sh
```
## Examples

A service intended to provide an email like feature is available at `examples/email`. This is meant to show how Pkv can be used. You can find more information about how to use the service by reading the [doc](./examples/email/README.md).

## FAQ

> [!TIP]
> Why haven't resource deletion RPC calls been included?
>
> The availability of such calls could depend on the design goals. Pkv operators are not trusted entities. They might mark a resource as deleted while still secretly storing it. When transitioning from the traditional assumption of trusted parties to the Prex model, it's important to explicitly raise these concerns.

## Roadmap

There are several directions to improve the Pkv service.

- [ ] Support more redis operations
  - [ ] Support all redis read operations under the account namespace
- [ ] Better reader protection
  - [ ] Support cheque check to let the sender pay for spam filtering
- [ ] Documentation
  - [ ] Provide more examples
  - [ ] Add references
- [ ] Refactor
  - [ ] Use the same set of quota and auth library for Prex and Pkv
