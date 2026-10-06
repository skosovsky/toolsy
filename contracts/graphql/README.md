# GraphQL contract adapter

The adapter targets the [GraphQL September 2025](https://spec.graphql.org/September2025/) query/mutation and introspection contract.
Discovery uses a finite type-reference selection (16 wrappers); truncated references
fail construction. Input projection supports built-in String, ID, Int (signed 32-bit),
Float (finite IEEE 754 double), Boolean, enums, lists, nullable values, argument defaults and finite input objects.
ID accepts strings and integer inputs. Non-null with a default may be omitted but may
not be null. OneOf input objects, unknown/custom scalars and recursive input objects are rejected explicitly.
List input accepts arrays or a single value following GraphQL's list coercion.

Hosts configure object results with `Options.Selections`, keyed by `query.field` or
`mutation.field`. A selection is a tree of field names (`Selection{Name, Fields}`),
not a query fragment. Only concrete OBJECT results and fields without required arguments
are supported; aliases, directives, fragments, union/interface results and custom scalars
are rejected. Scalar/enum outputs must have no selection. The adapter builds a static
query; model input is sent only as variables. Object fields must be explicitly selected
and produce their useful values. Selection and schema traversal are bounded to 16 levels
and 4096 nodes; invalid or unsupported shapes fail before publishing any tools.

Results contain the JSON value of the selected root field, validated against the projected
output contract. GraphQL errors fail execution (including partial-data responses). Missing
root data and oversized/invalid JSON fail execution without publishing sliced JSON.
Introspection and execution responses default to a 512 KiB byte bound. Host credentials,
endpoint authorization and business permissions remain host responsibilities. Discovery
publishes query/mutation names deterministically; subscriptions are unsupported.
