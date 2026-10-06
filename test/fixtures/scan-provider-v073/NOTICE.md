# Native scanner v0.7.3 acceptance

The retained native-before-proof records actual delegation through the published,
checksum-verified darwin/arm64 v0.7.3 scanner. The fixture labeled clean emitted
seven findings, and `TestProviderContract_Confighub/ScanFile_CleanYAML` genuinely
failed (7 versus expected 0). The misconfigured fixture emitted ten findings.
These are authored YAML inputs with actual released scanner execution, not live
cluster/controller evidence. Provider rules and expected zero findings are kept;
#797 strengthens the clean fixture's security/anti-affinity/token baseline.

Private downloaded binaries, credentials and logs are not committed. Actual
provider access used the existing local gh credential without exporting it.
Linux full CI still requires its separately scoped release-read secret (#774).
The file-only fixture does not establish functional runtime/application health.

The native-after-proof passes at source `27aa0167`: the strengthened clean
fixture has zero findings; the unchanged misconfigured fixture retains ten.
Both actual provider invocations are required, with provider archive and binary
hashes retained. The real provider contract (native and legacy) and file CLI
clean goldens pass. This is local native provider admission; Linux/full CI and
live scanner/controller acceptance remain separate required gates.
