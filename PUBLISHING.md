# Publishing scompress on GitHub

This repository is prepared for a first public GitHub push and automated Linux x86-64 releases.

## 1. Create an empty GitHub repository

On GitHub, create a repository named `scompress`. Choose **Public** if you want anyone to download and contribute. Do not initialize it with a README, `.gitignore`, or license: this local repository already contains those files.

## 2. Authenticate Git on this computer

Being signed in to GitHub in a browser does not necessarily authenticate the Git command-line client.

Check whether GitHub CLI is installed:

```bash
gh --version
```

If it is not installed, install GitHub CLI using the official instructions for Ubuntu: https://github.com/cli/cli/blob/trunk/docs/install_linux.md

Then authenticate and configure Git to use that login:

```bash
gh auth login --web --git-protocol https
gh auth setup-git
gh auth status
```

## 3. Set the canonical GitHub module and links

From the repository root, get the authenticated GitHub username:

```bash
GITHUB_USER="$(gh api user --jq .login)"
printf 'GitHub account: %s\n' "$GITHUB_USER"
```

Set the Go module path and replace the README / citation placeholders:

```bash
go mod edit -module "github.com/${GITHUB_USER}/scompress"
sed -i "s/YOUR_GITHUB_USERNAME/${GITHUB_USER}/g" README.md CITATION.cff
```

Review the result:

```bash
grep -n 'github.com/' README.md CITATION.cff
go list -m
```

## 4. Initialize and push the source repository

Make sure you are in the intended project directory:

```bash
pwd
ls -la
```

If this directory is not already a Git repository, initialize it. If it is already initialized, keep its history and rename the current branch to `main`:

```bash
if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  git init -b main
else
  git branch -M main
fi
```

Check your commit identity. If either value is empty, set it (use an email address verified on GitHub, or your GitHub-provided no-reply address):

```bash
git config user.name
git config user.email
# If needed:
# git config user.name "Shoaib"
# git config user.email "YOUR_VERIFIED_OR_GITHUB_NOREPLY_EMAIL"
```

Stage and inspect the files before committing:

```bash
git add .
git status --short
git diff --cached --stat
```

Confirm that no private files, credentials, personal images, generated binaries, or unrelated projects are staged. Build outputs under `dist/linux-amd64/` are ignored intentionally.

Commit and connect the empty GitHub repository:

```bash
git commit -m "Initial scompress 2.4.2 release"
git remote add origin "https://github.com/${GITHUB_USER}/scompress.git"
git remote -v
git push -u origin main
```

If `origin` already exists, use `git remote set-url origin "https://github.com/${GITHUB_USER}/scompress.git"` instead of `git remote add`.

## 5. Add the final application screenshot

Save the screenshot as:

```text
docs/images/scompress-main.png
```

The root README currently has a commented screenshot line. Uncomment it, then commit and push the screenshot and README update:

```bash
git add README.md docs/images/scompress-main.png
git commit -m "Add desktop application screenshot"
git push
```

## 6. Check continuous integration

Open the repository's **Actions** tab. The CI workflow runs Go tests, race tests, vet, formatting checks, the GTK build, and an installation smoke test on Ubuntu 24.04. Wait for the `CI` workflow on `main` to pass before publishing the first release.

## 7. Publish v2.4.2

Confirm `VERSION`, `CITATION.cff`, the CLI version, and the GUI version all say `2.4.2`. Then create and push the release tag:

```bash
git tag -a v2.4.2 -m "scompress 2.4.2"
git push origin v2.4.2
```

The release workflow builds the binaries on Ubuntu 24.04 and publishes these assets automatically:

- `scompress-2.4.2-linux-amd64.zip` — ready-to-install package with CLI, GUI, installer, uninstaller, README, user guide, license, and third-party notices.
- `scompress-2.4.2-linux-amd64.tar.gz` — the same ready-to-install package in TAR.GZ format.
- `SHA256SUMS.txt` — checksums for the ready-to-install ZIP and TAR.GZ.

GitHub automatically provides its standard source ZIP and TAR.GZ for the tag, so the release does not upload a duplicate source archive.

## Repository settings to consider

- Enable Issues for bug reports and feature requests.
- Enable Discussions if you want community questions and ideas.
- Enable private vulnerability reporting.
- Enable automatic deletion of head branches after pull requests are merged.
- Require the CI check before merging pull requests.

## License

The project currently includes the MIT License. Review it before making the repository public. The statically linked libwebp component retains its BSD 3-Clause notice in `THIRD_PARTY_NOTICES.txt`.
