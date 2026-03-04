# Release Process

This document describes the cub-scout release process and requirements.

## Release Artifacts

Every tagged release automatically produces:

1. **GitHub Release** with binaries
   - linux/amd64, linux/arm64
   - darwin/amd64, darwin/arm64
   - checksums.txt

2. **Homebrew Formula**
   - Automatically pushed to `confighub/homebrew-tap`
   - Enables: `brew install confighub/tap/cub-scout`

3. **Docker Images**
   - `ghcr.io/confighub/cub-scout:<version>`
   - `ghcr.io/confighub/cub-scout:latest`

## How to Release

1. **Ensure main is clean**
   ```bash
   git checkout main
   git pull
   go test ./...  # All tests must pass
   ```

2. **Create and push tag**
   ```bash
   git tag -a v0.X.Y -m "v0.X.Y - <release summary>"
   git push origin v0.X.Y
   ```

3. **Verify release**
   - Check GitHub Actions: Release workflow should complete
   - Verify GitHub Release page has all artifacts
   - Verify Homebrew: `brew update && brew info confighub/tap/cub-scout`

## Homebrew Requirements

**All releases MUST be available via Homebrew.**

The release workflow automatically updates the homebrew tap when:
- A new version tag is pushed
- The release workflow completes successfully

### Verifying Homebrew Release

After a release:
```bash
brew update
brew info confighub/tap/cub-scout  # Should show new version
brew upgrade cub-scout             # Should install new version
```

### Fixing a Broken Release

If a release needs to be re-cut (e.g., CI fix):

1. **Delete the broken tag**
   ```bash
   git tag -d v0.X.Y
   git push origin :refs/tags/v0.X.Y
   ```

2. **Fix the issue and re-tag**
   ```bash
   # Make fixes, commit, push
   git tag -a v0.X.Y -m "v0.X.Y - <release summary>"
   git push origin v0.X.Y
   ```

3. **Verify Homebrew updates**
   - The re-release will push a new formula to homebrew-tap
   - Run: `brew update && brew info confighub/tap/cub-scout`

**Important:** Re-tagging the same version will update the homebrew formula
with the new SHA256 checksums automatically.

## Release Checklist

- [ ] All tests pass (`go test ./...`)
- [ ] Golden tests are current (no `-update` needed)
- [ ] docs/roadmap.md reflects current version status
- [ ] Version tag follows semver (vX.Y.Z)
- [ ] Release notes describe changes clearly
- [ ] After release: verify GitHub artifacts exist
- [ ] After release: verify `brew info confighub/tap/cub-scout` shows new version

## Troubleshooting

### Homebrew formula not updated

1. Check if `HOMEBREW_TAP_TOKEN` secret is set in GitHub
2. Check GoReleaser logs in release workflow
3. Check `confighub/homebrew-tap` repo for recent commits

### Release workflow fails

1. Check workflow logs for specific failure
2. Common issue: golden tests need binary built first
3. Fix issue, delete tag, re-tag (see "Fixing a Broken Release")

## Configuration

Release automation is configured in:
- `.goreleaser.yaml` - Build, archive, homebrew, docker settings
- `.github/workflows/release.yaml` - GitHub Actions workflow
