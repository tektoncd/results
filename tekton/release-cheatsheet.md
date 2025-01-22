# Tekton Results Official Release Cheat Sheet

These steps provide a no-frills guide to performing an official release
of Tekton Results. To follow these steps you'll need a checkout of
the results repo, a terminal window and a text editor.

1. [Setup a context to connect to the dogfooding cluster](#setup-dogfooding-context) if you haven't already.

1. `cd` to root of Results git checkout.

1. Make sure the release `Pipeline` is up-to-date on the
   cluster.

   - [results-release](https://github.com/tektoncd/results/blob/main/tekton/release.yaml)

     This uses [ko](https://github.com/google/ko) to build all container images we release and generate the `release.yaml`
     ```shell script
     kubectl apply -f tekton/release.yaml
     ```

1. Select the commit you would like to build the release from, most likely the
   most recent commit at https://github.com/tektoncd/results/commits/main
   and note the commit's hash.

1. Create environment variables for bash scripts in later steps.

    ```bash
    VERSION_TAG=# UPDATE THIS. Example: v0.5.0
    RELEASE_GIT_SHA=# SHA of the release to be released
    ```

1. Confirm commit SHA matches what you want to release.

    ```bash
    git show $RELEASE_GIT_SHA
    ```

1. Create a workspace template file:

   ```bash
   cat <<EOF > workspace-template.yaml
   spec:
     accessModes:
     - ReadWriteOnce
     resources:
       requests:
         storage: 1Gi
   EOF
   ```

1. Execute the release pipeline.


    ```bash
        tkn --context dogfooding pipeline start results-release \
        --serviceaccount=results-release \
        --param=revision="${RELEASE_GIT_SHA}"  \
        --param=version="${VERSION_TAG}" \
        --param=docker_repo=ghcr.io/tektoncd/results \
        --param=bucket=gs://tekton-releases/results \
        --workspace name=release-secret,secret=ghcr-creds \
        --workspace  name=ws,volumeClaimTemplateFile=workspace-template.yaml
    ```

1. Watch logs of result-release.

1. Once the pipeline run is complete, check its results:

   ```bash
   tkn --context dogfooding pr describe <pipeline-run-name>

   (...)
   📝 Params

   NAME                    VALUE
   revision                 6ea31d92a97420d4b7af94745c45b02447ceaa19

   (...)
   ```

   The `commit-sha` should match `$RELEASE_GIT_SHA`.
   The two URLs can be opened in the browser or via `curl` to download the release manifests.

    1. The YAMLs are now released! Anyone installing Tekton Results will now get the new version. Time to create a new GitHub release announcement:

    1. Create additional environment variables

    ```bash
    OLD_VERSION=# Example v0.4.0
    TEKTON_PACKAGE=tektoncd/results
    ```


    1. Execute the Draft Release task.

    ```bash
    tkn --context dogfooding pipeline start   \
    --workspace name=shared,volumeClaimTemplateFile=workspace-template.yaml    \
    --workspace name=credentials,secret=release-secret  \
    -p package="${TEKTON_PACKAGE}"     -p git-revision="${RELEASE_GIT_SHA}"  \
    -p release-tag="${VERSION_TAG}"     -p previous-release-tag="${OLD_VERSION}"   \
    -p release-name="Tekton Results"     -p bucket="gs://tekton-releases/results"   \
    -p rekor-uuid="$REKOR_UUID"  release-draft
    ```

    1. Watch logs of create-draft-release

    1. On successful completion, a URL will be logged. Visit that URL and look through the release notes.
      1. Manually add upgrade and deprecation notices based on the generated release notes
      1. Double-check that the list of commits here matches your expectations
         for the release. You might need to remove incorrect commits or copy/paste commits
         from the release branch. Refer to previous releases to confirm the expected format.
    1. Un-check the "Set as a pre-release" checkbox since you're making a legit for-reals release!

    1. Publish the GitHub release once all notes are correct and in order.
    1. Mark the release as "the default release" if it's not a patch release or the patch release of the latest release.



1. Announce the release in Slack channels #general and #results.

Congratulations, you're done!

## Setup dogfooding context

1. Configure `kubectl` to connect to
   [the dogfooding cluster](https://github.com/tektoncd/plumbing/blob/main/docs/dogfooding.md):

    ```bash
    gcloud container clusters get-credentials dogfooding --zone us-central1-a --project tekton-releases
    ```

1. Give [the context](https://kubernetes.io/docs/tasks/access-application-cluster/configure-access-multiple-clusters/)
   a short memorable name such as `dogfooding`:

   ```bash
   kubectl config rename-context gke_tekton-releases_us-central1-a_dogfooding dogfooding
   ```

## Important: Switch `kubectl` back to your own cluster by default.

```bash
kubectl config use-context my-dev-cluster
```
