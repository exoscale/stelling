# CEL Authorizer Schema
This package contains the protobuf definition of the data used in the CEL authorizer.

It describes the fields you have available for use in the CEL expression.

When you update the schema, you must regenerate the associated go code:

```console
docker run -v "$(pwd):/workspace" --workspace "/workspace" --pull always bufbuild/buf generate
```