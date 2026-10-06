# Genuine restricted-viewer claim reads

These unedited outputs come from the isolated ConfigHub/cub v0.8.3 server.
The capture manifest records exact versions, source and binary hashes. No
credential, worker secret, private key or complete token is included. The small
decoded token identity subset is retained alongside the server's actual allowed
reads and rejected write; it is not an independent token-signature verifier.

The harness created an owned Space and server-hosted worker with organization
role `viewer`, and temporarily granted its bot user only `View` and
`ViewChildren` on the existing owned contract Space. The same viewer reads
revision 2 and its three direct claims through cub, Scout MCP, trace, explain
and receipt verification. The receipt retains its exact ConfigHub subject and
passes actual fingerprint validation. Setup and the attempted write were cub
test-harness operations outside Scout; Scout made only reads.

The viewer's metadata update is rejected with HTTP 403, and the Unit object is
unchanged. Removing the two read grants makes revision resolution fail; MCP
reports an error and explain omits claims with an explicit reason. That refusal
does not establish an empty claim set. Active permissions were restored and the
owned worker, Space and Kubernetes namespace were removed.

Three attempts remain at the manifest's private paths. The first compared a
Unit response envelope including the deliberately changed Space permissions;
that harness comparison failed while the Unit stayed unchanged. The comparison
was repaired to check the Unit itself and active permission principals. The
second passed scoped read/write denial, and the third also passed CLI/receipt
projections. Credentials and setup outputs remain private.

This closes the named viewer read/write/permission-removal checkpoint. It does
not establish effective or inherited claim coverage, per-entity filtered list
completeness, evaluated governance, all user roles or final release acceptance.
