# Examples

Here, an email-like service is provided to illustrate how Pkv can be utilized.

## Setup

To test the service, start by configuring mock services:
```bash
# OAuth client
go run cmd/genjwt/genjwt.go
# Mock OIDC provider, or you may set IS_TEST=false and use google
go run cmd/mock_op/mock_op.go
# Mock Prex
go run cmd/mock_prex/mock_prex.go
# Main Pkv service
go run cmd/pkv/pkv.go
```

A Redis service will be set up automatically if you use the devcontainer included in this repository.

> [!TIP]
> For a more cost-effective storage solution in a production environment, consider using a disk-based Redis service like [kvrocks](https://github.com/apache/kvrocks).

## Write an email

To send an email from `alice@op1.com` to `bob@op2.com`, execute the following command:

```bash
go run examples/email/main.go write --messager=traditional
```

To ensure a permissionless service, no account registration is required for writing. All that is needed is a quota token to cover the usage costs.

> [!TIP]
> Enabling unrestricted writes aligns with the email model, where anyone can contact anyone. However, this also raises the risk of spam emails. One possible solution is to require each sender to include a Prex cheque, which covers the cost for the recipient to run a spam filter model on a device or service it trusts. This feature could be added in the future.

To verify the success of the operation, you can examine the Redis service. Upon success, it should display output similar to the following:

```bash
$ redis-cli
127.0.0.1:6379> XRANGE accounts/bob@op2.com/streams/email-stream-1-v0.1.0 - +
1) 1) "1757230596210-0"
   2) 1) "value"
      2) "\x1a\tlocalhost \x83\x87\x03*\ralice@op1.com2\x0bbob@op2.com:Bvalues/bafkreie3b5546eam3wbpfqyfdiejjs7zcadmgntaisffd2f5yodr2zisxa"
2) 1) "1757230596211-0"
   2) 1) "value"
      2) "\x1a\tlocalhost \x83\x87\x03*\ralice@op1.com2\x0bbob@op2.com:Bvalues/bafkreiawp7k5cyer7ndowgwrxp57kl7oawlx2ddctnpdj3otx3jdkdqzqi"
```

## Login to read emails

To retrieve an email, authentication is required since you would not want unauthorized access to your messages. In Pkv, users are only permitted to read values within their own namespaces. All resources have the prefix `accounts/${account_id}`, where `account_id` is a string that identifies the user. One option is to use the email address from an OIDC provider trusted by the operator.

> [!TIP]
> If a user prefers not to reveal their email, they can use the [ZKLogin feature provided by Sui](https://sui.io/zklogin), which can be adapted into another independent PAID service. This approach aligns with the [purchasable privacy](https://github.com/atticplaygroup/prex/wiki/paid-service#motivation) design of PAID services.

~~A specific account, the `guest` account, allows anyone to log in as it. To obtain the JWT token for this account, access `http://localhost:8080/v1/guest`. With this token, anyone can share data publicly on Pkv.~~
All resources are public on pkv. Only stream resources are guarded by the authentication wall. Always encrypt the message before you upload. Don't use the traditional messager in production as everyone purchasing a valid session token is allowed to download.

To login as other users:
```bash
$ go run examples/email/main.go login
Please open http://localhost:8080/v1/login and after login paste the access token here: 
```

Navigate to the login page and complete the OAuth process, then copy the JWT token here. Alternatively, create a YAML file at `${HOME}/.prex/pmail-config.yaml` with the JWT:
```yaml
account:
    auth_token: eyJhbGciOiJIUzI1Ni...
```

If using the mock operation, a fixed JWT for `bob@op2.com` will be issued, which can be used for testing as Bob.

## Read emails

```bash
$ go run examples/email/main.go read --messager=traditional
[0] email host:"localhost"  port:50051  sender:"alice@op1.com"  recipient:"bob@op2.com"  content_resource_name:"values/bafkreie3b5546eam3wbpfqyfdiejjs7zcadmgntaisffd2f5yodr2zisxa":
Subject: first message

Dear Bob, This is my first message

[1] email host:"localhost"  port:50051  sender:"alice@op1.com"  recipient:"bob@op2.com"  content_resource_name:"values/bafkreiawp7k5cyer7ndowgwrxp57kl7oawlx2ddctnpdj3otx3jdkdqzqi":
Subject: second message

Dear Bob, This is my second message
```

The `messager` argument allows users to choose how their data is stored on Pkv. If a user trusts Pkv for access control and the OIDC provider, they can opt for the traditional messager, which mirrors conventional email services by having the operator manage both metadata and email content.

If a user does not trust the service to safeguard email content, they can use the `pgp_e2ee` messager, which employs PGP to encrypt and sign messages. However, the user must handle key management independently. Metadata will still be in plaintext on the Pkv service, although it remains protected by access controls.

```bash
$ go run examples/email/main.go write --messager=pgp_e2ee
Successfully sent email, resource names: values/bafkreihgtxhoi4bojunggrptuoeszfewq4apvxmnowz4g3rrnhm7gakdia and values/bafkreigai3e5wzqhagmk6kmethqnoplrzl2yqcw3fqwhu56oowi4xznov4
$ go run examples/email/main.go read --messager=pgp_e2ee
[0] email host:"localhost"  port:50051  sender:"alice@op1.com"  recipient:"bob@op2.com"  content_resource_name:"values/bafkreihgtxhoi4bojunggrptuoeszfewq4apvxmnowz4g3rrnhm7gakdia":
Subject: first message

Dear Bob, This is my first message

[1] email host:"localhost"  port:50051  sender:"alice@op1.com"  recipient:"bob@op2.com"  content_resource_name:"values/bafkreigai3e5wzqhagmk6kmethqnoplrzl2yqcw3fqwhu56oowi4xznov4":
Subject: second message

Dear Bob, This is my second message
```

If a user wishes to further conceal metadata leakage and access patterns, they can select a user agent that implements an oblivious storage protocol, such as a fully oblivious storage like [ORAM](https://eprint.iacr.org/2013/280) or one secure against [passive leakages](https://eprint.iacr.org/2013/280). Only the messager component on the client side needs modification; Pkv itself does not require changes, theoretically (unless a server-side TEE is added to enhance colocation and speed).

> [!TIP]
> This illustrates the advantage of [decoupling user agents and PAID services](https://github.com/atticplaygroup/prex/wiki/paid-service#decoupled). Users can switch user agents while still utilizing the same key-value storage. The operator remains unaware of these changes. PAID services extend the benefits of microservices beyond a single company to a global scale.

> [!TIP]
> Why store encrypted contents ~~under the `guest` account~~ publicly instead of a protected account location?
>
> In addition to the advantage of potentially choosing another Pkv provider for metadata and content storage to ensure data liveness, public storage can sometimes be more cost-effective than storage managed by a single entity. This is because content availability can be bootstrapped using storage media with higher data loss risks but lower costs from other parties, facilitated by erasure codes and insurance contracts. With a single stakeholder, it is not possible to bootstrap availability across the globe.
