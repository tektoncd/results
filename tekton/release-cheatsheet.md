# Tekton Results Official Release Cheat Sheet

These steps provide a no-frills guide to performing an official release
of Tekton Results. To follow these steps you'll need a checkout of
the results repo, a terminal window and a text editor.

1. [Setup a context to connect to the dogfooding cluster](#setup-dogfooding-context) if you haven't already.

1. Install the [rekor CLI](https://docs.sigstore.dev/rekor/installation/) if you haven't already.

1. [Install kustomize](https://kubectl.docs.kubernetes.io/installation/kustomize) if you haven't already.

1. `cd` to root of Results git checkout.

1. Select the commit you would like to build the release from (NOTE: the commit is full (40-digit) hash.)
    - Select the most recent commit on the ***main branch*** if you are cutting a major or minor release i.e. `x.0.0` or `0.x.0`
    - Select the most recent commit on the ***`release-<version number>x` branch***, e.g. [`release-v0.47.x`](https://github.com/tektoncd/results/tree/release-v0.47.x) if you are patching a release i.e. `v0.47.2`.

1. Create a `release.env` file with environment variables for bash scripts in later steps, and source it:

    ```bash
    cat <<EOF > release.env
    TEKTON_VERSION= # Example: v0.19.0
    TEKTON_RELEASE_GIT_SHA= # SHA of the release to be released, e.g. 5b082b1106753e093593d12152c82e1c4b0f37e5
    TEKTON_OLD_VERSION= # Example: v0.18.0
    TEKTON_PACKAGE=tektoncd/results
    TEKTON_REPO_NAME=results
    EOF
    . ./release.env
    ```

1. Confirm commit SHA matches what you want to release.

    ```bash
    git show $TEKTON_RELEASE_GIT_SHA
    ```

1. Create a workspace template file:

   ```bash
   WORKSPACE_TEMPLATE=$(mktemp /tmp/workspace-template.XXXXXX.yaml)
   cat <<'EOF' > $WORKSPACE_TEMPLATE
   spec:
    accessModes:
    - ReadWriteOnce
    resources:
      requests:
        storage: 1Gi
   EOF
   ```

1. Execute the release pipeline (take ~45 mins).
    
    **The minimum required tkn version is v0.30.0 or later**

    **If you are back-porting include this flag: `--param=releaseAsLatest="false"`**

   ```bash
    tkn --context dogfooding pipeline start tekton-results-release \
      --filename=tekton/results-release-pipeline.yaml \
      --param package=github.com/tektoncd/results \
      --param repoName="${TEKTON_REPO_NAME}" \
      --param gitRevision="${TEKTON_RELEASE_GIT_SHA}" \
      --param imageRegistry=ghcr.io \
      --param imageRegistryPath=tektoncd/results \
      --param imageRegistryRegions="" \
      --param imageRegistryUser=tekton-robot \
      --param serviceAccountImagesPath=credentials \
      --param versionTag="${TEKTON_VERSION}" \
      --param releaseBucket=tekton-releases \
      --param koExtraArgs="" \
      --workspace name=release-secret,secret=oci-release-secret \
      --workspace name=release-images-secret,secret=ghcr-creds \
      --workspace name=workarea,volumeClaimTemplateFile="${WORKSPACE_TEMPLATE}" \
      --tasks-timeout 2h \
      --pipeline-timeout 3h
   ```

    Accept the default values of the parameters (except for "releaseAsLatest" if backporting).

1. Watch logs of results-release.

1. Once the pipeline is complete, check its results:

    ```bash
    tkn --context dogfooding pr describe <pipeline-run-name>

    (...)
    📝 Results

    NAME                    VALUE
    ∙ commit-sha            ff6d7abebde12460aecd061ab0f6fd21053ba8a7
    ∙ release-file           https://infra.tekton.dev/tekton-releases/results/previous/v0.13.0/release.yaml
    ∙ release-file-no-tag    https://infra.tekton.dev/tekton-releases/results/previous/v0.13.0/release.notags.yaml
    (...)
    ```

    The `commit-sha` should match `$TEKTON_RELEASE_GIT_SHA`.
    The two URLs can be opened in the browser or via `curl` to download the release manifests.


1. The YAMLs are now released! Anyone installing Tekton Results will get the new version. Time to create a new GitHub release announcement:

    1. Find the Rekor UUID for the release

    ```bash
    RELEASE_FILE=https://infra.tekton.dev/tekton-releases/results/previous/${TEKTON_VERSION}/release.yaml
    WATCHER_IMAGE_SHA=$(curl -L $RELEASE_FILE | sed -n 's/"//g;s/.*ghcr\.io.*watcher.*@//p;')
    REKOR_UUID=$(rekor-cli search --sha $WATCHER_IMAGE_SHA | grep -v Found | head -1)
    echo -e "WATCHER_IMAGE_SHA: ${WATCHER_IMAGE_SHA}\nREKOR_UUID: ${REKOR_UUID}"
    ```

    1. Execute the Draft Release Pipeline.

        Create a pod template file:

        ```shell
        POD_TEMPLATE=$(mktemp /tmp/pod-template.XXXXXX.yaml)
        cat <<'EOF' > $POD_TEMPLATE
        securityContext:
          fsGroup: 65532
          runAsUser: 65532
          runAsNonRoot: true
        EOF
        ```

        ```shell
        tkn pipeline start \
          --workspace name=shared,volumeClaimTemplateFile="${WORKSPACE_TEMPLATE}" \
          --workspace name=credentials,secret=oci-release-secret \
          --pod-template "${POD_TEMPLATE}" \
          -p package="${TEKTON_PACKAGE}" \
          -p git-revision="$TEKTON_RELEASE_GIT_SHA" \
          -p release-tag="${TEKTON_VERSION}" \
          -p previous-release-tag="${TEKTON_OLD_VERSION}" \
          -p release-name="${TEKTON_RELEASE_NAME}" \
          -p repo-name="${TEKTON_REPO_NAME}" \
          -p bucket="tekton-releases" \
          -p rekor-uuid="${REKOR_UUID}" \
          release-draft-oci
        ```

    1. Watch logs of resulting pipeline run on pipeline `release-draft-oci`

    1. On successful completion, a URL will be logged. Visit that URL and look through the release notes.
      1. Manually add upgrade and deprecation notices based on the generated release notes
      1. Double-check that the list of commits here matches your expectations
         for the release. You might need to remove incorrect commits or copy/paste commits
         from the release branch. Refer to previous releases to confirm the expected format.

    1. Un-check the "This is a pre-release" checkbox since you're making a legit for-reals release!

    1. Publish the GitHub release once all notes are correct and in order.

1. Create a branch for the release named `release-<version number>x`, e.g. [`release-v0.28.x`](https://github.com/tektoncd/results/tree/release-v0.28.x)
   and push it to the repo https://github.com/tektoncd/results.
   (This can be done on the Github UI.)
   Make sure to fetch the commit specified in `TEKTON_RELEASE_GIT_SHA` to create the released branch.
   > Background: The reason why we need to create a branch for the release named `release-<version number>x` is for future patch releases. Cherrypicked PRs for the patch release will be merged to this branch. For example, [v0.47.0](https://github.com/tektoncd/results/releases/tag/v0.47.0) has been already released, but later on we found that an important [PR](https://github.com/tektoncd/results/pull/6622) should have been included to that release. Therefore, we need to do a patch release i.e. v0.47.1 by [cherrypicking this PR](https://github.com/tektoncd/results/pull/6622#pullrequestreview-1414804838), which will trigger [tekton-robot to create a new PR](https://github.com/tektoncd/results/pull/6622#issuecomment-1539030091) to merge the changes to the [release-v0.47.x branch](https://github.com/tektoncd/results/tree/release-v0.47.x).

1. If the release introduces a new minimum version of Kubernetes required,
   edit `README.md` on `main` branch and add the new requirement with in the
   "Required Kubernetes Version" section

1. Edit `releases.md` on the `main` branch, add an entry for the release.
   - In case of a patch release, replace the latest release with the new one,
     including links to docs and examples. Append the new release to the list
     of patch releases as well.
   - In case of a minor or major release, add a new entry for the
     release, including links to docs and example
   - Check if any release is EOL, if so move it to the "End of Life Releases"
     section

1. Push & make PR for updated `releases.md` and `README.md`

1. Test release that you just made against your own cluster (note `--context my-dev-cluster`):

    ```bash
    # Test latest
    kubectl --context my-dev-cluster apply --filename https://infra.tekton.dev/tekton-releases/results/latest/release.yaml
    ```

    ```bash
    # Test backport
    kubectl --context my-dev-cluster apply --filename https://infra.tekton.dev/tekton-releases/results/previous/v0.11.2/release.yaml
    ```

1. Announce the release in Slack channels #general, #announcements and #results.

1. Update [the catalog repo](https://github.com/tektoncd/catalog) test infrastructure
to use the new release by updating the test matrix in the `[ci.yaml](https://github.com/tektoncd/catalog/blob/main/.github/workflows/ci.yaml)`.

1. Update [the plumbing repo](https://github.com/tektoncd/plumbing/blob/main/tekton/cd/results/base/kustomization.yaml#L2) to deploy the latest version to the dogfooging cluster on OCI.

1. For major releases, the [website sync configuration](https://github.com/tektoncd/website/blob/main/sync/config/pipelines.yaml)
   to include the new release.

Congratulations, you're done!

## Setup dogfooding context

1. Configure `kubectl` to connect to
   [the dogfooding cluster](https://github.com/tektoncd/plumbing/blob/main/docs/dogfooding.md):

   The dogfooding cluster is currently an OKE cluster in oracle cloud. we need the Oracle Cloud CLI client. Install oracle cloud cli (https://docs.oracle.com/en-us/iaas/Content/API/SDKDocs/cliinstall.htm) 

    ```bash
    oci ce cluster create-kubeconfig --cluster-id <CLUSTER-OCID> --file $HOME/.kube/config --region <CLUSTER-REGION> --token-version 2.0.0  --kube-endpoint PUBLIC_ENDPOINT
    ```

1. Give [the context](https://kubernetes.io/docs/tasks/access-application-cluster/configure-access-multiple-clusters/)
   a short memorable name such as `dogfooding`:

   ```bash
   kubectl config current-context
   ```
   get the context name and replace with current_context_name

   ```bash
   kubectl config rename-context <current_context_name> dogfooding
   ```

1. **Important: Switch `kubectl` back to your own cluster by default.**

    ```bash
    kubectl config use-context my-dev-cluster
    ```
