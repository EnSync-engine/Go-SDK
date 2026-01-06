# Versioning Guide

This document explains how versioning works in the EnSync Go SDK project and how to release new versions.

## Semantic Versioning

We follow [Semantic Versioning](https://semver.org/) (SemVer):
- **MAJOR**: Breaking changes that require users to update their code
- **MINOR**: New features that are backwards compatible
- **PATCH**: Bug fixes and internal improvements

## Version Management

### Using Makefile (Recommended)

## [Unreleased]

### Fixed
- Fixed type mismatch in subscription metadata (`map[string]interface{}` -> `MessageMetadata`).
- Restored error tracking in `restoreSubscriptions` for WebSocket engine to ensure failures are reported.

### Changed
- Refactored WebSocket internal protocol strings to constants.


The easiest way to manage versions is using the provided Makefile targets:

```bash
# Show current version
make version-current

# Bump versions
make version-patch   # 1.0.0 -> 1.0.1
make version-minor   # 1.0.1 -> 1.1.0  
make version-major   # 1.1.0 -> 2.0.0

# Get help
make version-help
```

### Using the Version Script Directly

You can also use the version script directly:

```bash
# Show current version
./scripts/version.sh current

# Bump versions
./scripts/version.sh patch
./scripts/version.sh minor
./scripts/version.sh major
```

## Automated Release Process

Our automated release system works as follows:

1. **Make your changes** and ensure all tests pass
2. **Update documentation** if needed
3. **Bump the version** using `make version-patch|minor|major`
4. **Commit and push** your changes to the `main` branch
5. **GitHub Actions automatically**:
   - Runs tests
   - Creates a git tag
   - Publishes a GitHub release
   - Updates Go module registry

### What the Version Script Does

When you run a version bump command, the script:

1. Updates the version in `CHANGELOG.md`
2. Adds a new section for the version with today's date
3. Prompts you to add release notes
4. Shows a preview of changes
5. Allows you to commit the changes locally

### What GitHub Actions Does

When you push changes to `main` with a new version in the changelog:

1. **Auto-Tag Workflow** triggers
2. Runs tests to ensure quality
3. Detects the new version from `CHANGELOG.md`
4. Creates and pushes a git tag (e.g., `v1.2.3`)
5. Creates a GitHub release with:
   - Release notes from changelog
   - Installation instructions
   - Links to documentation

## Manual Releases

You can also trigger releases manually using GitHub's web interface:

1. Go to **Actions** → **Auto Tag and Release**
2. Click **Run workflow**
3. Enter the version (e.g., `v1.2.3`)
4. Choose whether to create a GitHub release
5. Click **Run workflow**

## Best Practices

### When to Bump Versions

- **Patch (1.0.0 → 1.0.1)**:
  - Bug fixes
  - Documentation updates
  - Internal refactoring
  - Performance improvements

- **Minor (1.0.0 → 1.1.0)**:
  - New features
  - New API methods
  - Deprecating (but not removing) functionality
  - Major internal improvements

- **Major (1.0.0 → 2.0.0)**:
  - Breaking API changes
  - Removing deprecated features
  - Major architectural changes
  - Changes that require user code updates

### Changelog Best Practices

- Write clear, user-focused release notes
- Group changes by type: Added, Changed, Fixed, Removed
- Include code examples for new features
- Reference issue numbers when applicable
- Use present tense ("Add" not "Added")

### Release Workflow

1. **Feature Development**:
   ```bash
   git checkout -b feature/new-feature
   # ... make changes ...
   git commit -m "feat: add new feature"
   git push origin feature/new-feature
   ```

2. **Create Pull Request**:
   - Ensure tests pass
   - Update documentation
   - Review changes

3. **After Merge to Main**:
   ```bash
   git checkout main
   git pull origin main
   make version-minor  # or patch/major as appropriate
   git add .
   git commit -m "chore: bump version to v1.2.0"
   git push origin main
   ```

4. **Automated Release**:
   - GitHub Actions creates tag and release automatically
   - Monitor the Actions tab for any issues

## Troubleshooting

### Version Script Issues

- **Permission denied**: Run `chmod +x scripts/version.sh`
- **No version found**: Ensure `CHANGELOG.md` exists and has proper format
- **Git errors**: Ensure you're in a git repository and have proper permissions

### GitHub Actions Issues

- **Tests fail**: Fix failing tests before the workflow can proceed
- **Tag already exists**: The workflow will skip if the tag already exists
- **Permission issues**: Ensure the repository has proper Actions permissions

## File Structure

```
.github/
  workflows/
    auto-tag.yml      # Automated tagging and release workflow
scripts/
  version.sh          # Semantic versioning helper script
CHANGELOG.md          # Version history and release notes
Makefile             # Build targets including version management
```

## Configuration

The automated release system uses these configuration points:

- **Go version**: Set in `.github/workflows/auto-tag.yml` (currently 1.24)
- **Branch**: Triggered on pushes to `main` branch
- **Permissions**: Requires `contents: write` permission for creating releases
- **Changelog format**: Expects `## [X.Y.Z] - YYYY-MM-DD` format

## Support

If you encounter issues with the versioning system:

1. Check the GitHub Actions logs
2. Ensure your git working directory is clean
3. Verify the changelog format is correct
4. Ask for help in project discussions or issues
