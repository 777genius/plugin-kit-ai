# Release-bound Directory bootstrap inputs

Stable release validation requires three exact files in this directory:

- `snapshot.json`: the byte-exact signed schema-1 snapshot;
- `envelope.json`: its detached Ed25519 envelope;
- `trusted-keys.json`: the reviewed schema-1 trust document containing the
  production key compiled into `agentplugins`.

They are deliberately absent until a real production publication exists. Do
not synthesize an initial sequence. From `cli/plugin-kit-ai`, generate the Go
bootstrap only from publication-ledger artifacts:

```sh
go run ./cmd/agentplugins/bootstrapgen \
  -snapshot cmd/agentplugins/directory_bootstrap_inputs/snapshot.json \
  -envelope cmd/agentplugins/directory_bootstrap_inputs/envelope.json \
  -trust cmd/agentplugins/directory_bootstrap_inputs/trusted-keys.json \
  -expected-key-id uap-directory-2026-01 \
  -expected-public-key HalXARjat+v3ylTPLMAnvuavRo4ZfrF+DbWwsjlp2bI= \
  -output cmd/agentplugins/directory_bootstrap_generated.go
```

The stable workflow reruns the same verification, checks byte-for-byte source
reproducibility, and rejects a snapshot that is not valid at release time.
