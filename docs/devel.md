# Developer documentation

Ignite is a Go project using well-known libraries like:

- github.com/spf13/cobra
- github.com/spf13/pflag
- k8s.io/apimachinery
- sigs.k8s.io/yaml
- github.com/firecracker-microvm/firecracker-go-sdk

and so on.

It uses Go modules as the vendoring mechanism.

## Build from source

The only build requirement is Docker.

To build `ignite`, `ignited` and `ignite-spawn` for all supported architectures, run:

```console
make build-all
```

To only build for a specific architecture, append the architecture to the command:

```console
make build-all-amd64
make build-all-arm64
```

## Pre-commit tidying

Before committing, please run this make target to (re)generate
autogenerated content and tidy your local environment:

```console
make autogen tidy
```

## Building reference OS images

```console
make -C images WHAT=ubuntu
make -C images WHAT=centos
```

## Generic instructions on releasing a version

- Fix all issues in the `v0.X.Y` milestone
- Create a `v0.X.Y` tracking issue (like https://github.com/weaveworks/ignite/issues/379)
- Update documentation for the latest version (like https://github.com/weaveworks/ignite/pull/378)
- Make sure your git remote `upstream` points to `git@github.com:weaveworks/ignite.git`, and `origin` to `git@github.com:<user>/ignite.git`
- Get a Github API token with `repo` access, and put it in `bin/gren_token` for automatic release note generation
- Make sure you're part of the Ignite DockerHub Team so you have access to push to the `weaveworks/ignite*` repositories

## Releasing a minor version

- A minor version is done based off the `main` branch
- Set the environment variable to tell what minor version to release: `export MINOR=X`
  - If this is a prerelease, set e.g. `export EXTRA=-alpha.1`, `export EXTRA=-beta.1`, or `export EXTRA=-rc.1`
- The script to run is `hack/minor-release.sh all`. It will:
  - Tidy your environment by running `make tidy autogen graph` and doing a commit
  - Autogenerate the changelog, provisionally using [GREN](https://github.com/github-tools/github-release-notes). The script will wait for you to open an editor and manually fixup `docs/releases/v0.X.0.md`. Then proceed with `Y`, which will create the commit to be tagged `v0.X.0`
  - Create the `v0.X.0` tag using `git tag`
  - Build the release binaries to `bin/releases/v0.X.0` and push the `weaveworks/ignite:v0.X.0` images and manifest list to Docker Hub
  - Push the tag, and latest commits to the `main` and newly-created `release-0.X` branch

## Releasing a patch version

- A patch version is done based off the `release-0.X` branch
  - **Note:** Before running the release, `git cherrypick` relevant commits into the release branch
- Set the environment variables to tell what patch version to release: `export MINOR=X` and `export PATCH=Y`
  - If this is a prerelease, set e.g. `export EXTRA=-alpha.1`, `export EXTRA=-beta.1`, or `export EXTRA=-rc.1`
- The script to run is `hack/patch-release.sh all`
  - Tidy your environment by running `make tidy autogen graph` and doing a commit
  - Autogenerate the changelog, provisionally using [GREN](https://github.com/github-tools/github-release-notes). The script will wait for you to open an editor and manually fixup `docs/releases/v0.X.Y.md`. Then proceed with `Y`, which will create the commit to be tagged `v0.X.Y`
  - Create the `v0.X.Y` tag using `git tag`
  - Build the release binaries to `bin/releases/v0.X.Y` and push the `weaveworks/ignite:v0.X.Y` images and manifest list to Docker Hub
  - Push the tag, and latest commits to the `release-0.X` branch

## Publishing a release

- Go to `Project Releases` in the Github UI, and select `Draft a new release`
- Select the tag you've just created `v0.X.Y` from either the `main` (minor releases) or `release-0.X` branch
- Let the release title be `v0.X.Y`
- Paste the content in `docs/releases/v0.X.Y.md` in the release description, and add installing guidelines as per the earlier releases
- Upload the binaries in `bin/releases/v0.X.Y` named as `{ignite,ignited}-{amd64,arm64}`
- Click `Publish Release` and announce to everyone you know!
