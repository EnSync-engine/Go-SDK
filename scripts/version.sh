#!/bin/bash

# Semantic Version Helper for EnSync Go SDK
# Usage: ./scripts/version.sh [major|minor|patch|current]

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Get current version from changelog
get_current_version() {
    grep -E "^## \[[0-9]+\.[0-9]+\.[0-9]+\]" CHANGELOG.md | head -1 | sed -E 's/^## \[([0-9]+\.[0-9]+\.[0-9]+)\].*/\1/' || echo "0.0.0"
}

# Parse semantic version
parse_version() {
    local version=$1
    IFS='.' read -r -a parts <<< "$version"
    MAJOR=${parts[0]:-0}
    MINOR=${parts[1]:-0}
    PATCH=${parts[2]:-0}
}

# Increment version
increment_version() {
    local type=$1
    local current=$(get_current_version)
    
    if [ "$current" = "0.0.0" ]; then
        echo "No version found in CHANGELOG.md"
        current="0.1.0"
    fi
    
    parse_version "$current"
    
    case $type in
        major)
            MAJOR=$((MAJOR + 1))
            MINOR=0
            PATCH=0
            ;;
        minor)
            MINOR=$((MINOR + 1))
            PATCH=0
            ;;
        patch)
            PATCH=$((PATCH + 1))
            ;;
        *)
            echo "Invalid version type. Use: major, minor, or patch"
            exit 1
            ;;
    esac
    
    echo "${MAJOR}.${MINOR}.${PATCH}"
}

# Update changelog with new version
update_changelog() {
    local new_version=$1
    local date=$(date +%Y-%m-%d)
    
    echo -e "${BLUE}Updating CHANGELOG.md with version $new_version${NC}"
    
    # Create backup    
    # Add new version section after [Unreleased]
    sed -i.tmp "/^## \[Unreleased\]/a\\
\\
## [$new_version] - $date\\
\\
### Added\\
- Version $new_version release\\
" CHANGELOG.md
    
    # Clean up temp file
    rm -f CHANGELOG.md.tmp
    
    echo -e "${GREEN}✓ CHANGELOG.md updated${NC}"
}

# Create git tag
create_tag() {
    local version=$1
    local tag="v$version"
    
    echo -e "${BLUE}Creating git tag $tag${NC}"
    
    if git rev-parse "$tag" >/dev/null 2>&1; then
        echo -e "${YELLOW}⚠ Tag $tag already exists${NC}"
        return 1
    fi
    
    git add CHANGELOG.md
    git commit -m "chore: bump version to $version

- Updated changelog with version $version
- Ready for release tagging"
    
    git tag -a "$tag" -m "Release $version

See CHANGELOG.md for details."
    
    echo -e "${GREEN}✓ Created tag $tag${NC}"
    echo -e "${YELLOW}Run 'git push origin main && git push origin $tag' to publish${NC}"
}

# Show current version info
show_current() {
    local current=$(get_current_version)
    echo -e "${BLUE}Current version information:${NC}"
    echo "Version: $current"
    
    if git rev-parse "v$current" >/dev/null 2>&1; then
        echo "Tag: v$current (exists)"
    else
        echo "Tag: v$current (not created)"
    fi
    
    echo ""
    echo -e "${BLUE}Available commands:${NC}"
    echo "$0 patch   # Increment patch version (x.y.Z)"
    echo "$0 minor   # Increment minor version (x.Y.0)"
    echo "$0 major   # Increment major version (X.0.0)"
    echo "$0 current # Show this information"
}

# Validate we're in the right directory
if [ ! -f "CHANGELOG.md" ] || [ ! -f "go.mod" ]; then
    echo -e "${RED}Error: Run this script from the project root directory${NC}"
    exit 1
fi

# Main logic
case "${1:-current}" in
    current)
        show_current
        ;;
    major|minor|patch)
        NEW_VERSION=$(increment_version "$1")
        echo -e "${GREEN}New $1 version: $NEW_VERSION${NC}"
        
        read -p "Update changelog and create tag? (y/N) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            update_changelog "$NEW_VERSION"
            create_tag "$NEW_VERSION"
        else
            echo "Cancelled"
        fi
        ;;
    *)
        echo -e "${RED}Invalid argument: $1${NC}"
        show_current
        exit 1
        ;;
esac
