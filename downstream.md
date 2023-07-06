# Managing downstream fork

The purpose of this document is to describe the manual steps required to synchronize the downstream fork with upstream, while keeping the downstream only commits on the top of the branch. It also covers the process of submitting downstream only changes.

## Requirements

1. The history should not be overwritten
2. Ability to manage downstream only changes
3. Downstream commits should be always on top of the upstream commits
4. Ability to review and run CI on all changes

## High level description

We use rebase to keep the history of the upstream commits intact with the downstream changes on top. We create a new downstream branch for each rebase with branch names using the pattern `downstream-X.Y.Z-NN` where `X.Y.Z` is the upstream version and `NN` is the number of downstream revisions/rebases. We squash the downstream commits for each rebase to create a single commit which is easier to carry over. For each rebase we create a PR with the purpose of CI and review, but those PRs are never merged, and instead the new rebase branch is pushed to the downstream fork. The CI is still triggered the same way, on both pull_request and push.

## Initial configuration

In the most traditional case, the developer forked the upstream repository and cloned it locally. Two other remotes were added, `upstream` pointing to https://github.com/tektoncd/results and `downstream` pointing to https://github.com/openshift-pipelines/tektoncd-results. How to add those remotes is out of scope for this document. Given SSH is used for interaction with GitHub, `git remote -v` will return:

```
downstream      git@github.com:openshift-pipelines/tektoncd-results.git (fetch)
downstream      git@github.com:openshift-pipelines/tektoncd-results.git (push)
origin  git@github.com:<your-github-user>/results.git (fetch)
origin  git@github.com:<your-github-user>/results.git (push)
upstream        git@github.com:tektoncd/results.git (fetch)
upstream        git@github.com:tektoncd/results.git (push)
```

## Steps to manually sync with upstream

1. Sync the upstream branch: `git checkout main && git pull --ff-only upstream main`. This will ensure new changes from upstream are merged using the `Fast-forward` mechanism and no merge commit was created.
2. Fetch the branches from the downstream repo: `git fetch downstream` and find the most recent downstream branch (e.g. `downstream/downstream-0.5.0-01`)
3. Create a new branch from it: `git checkout -b <downstream-X.Y.Z-N(N+1)>  downstream/<downstream-X.Y.Z-NN>` (e.g. `git checkout -b downstream-0.5.0-02  downstream/downstream-0.5.0-01`)
4. If there are commits on top of the "[CARRY]: Tekton Results downstream configuration" commit, squash them preserving the "[CARRY]: Tekton Results downstream configuration" commit message. There should be only one downstream commit!
5. Rebase the main branch: `git rebase main`
6. Push the branch to your fork: `git push origin <downstream-X.Y.Z-NN>` (e.g. `git push origin downstream-0.5.0-02`)
7. Open a PR choosing base repository "openshift-pipeline/tektoncd-results" and base branch the latest downstream-X.Y.Z-NN branch (here downstream-0.5.0-01).
    1. Add "DO NOT MERGE, CI ONLY" prefix to the PR title
    2. In the description add and properly update the following fields:
        - New features:
        - Changes that require updates to our deployment configuration:
        - Breaking changes:
    3. Wait for the CI to run and make sure it passes
    4. Wait for reviews from the team members
8. When the PR is ready to be merged, instead of merging, close it and push the branch to the downstream repo: `git push downstream <downstream-X.Y.Z-NN>` (e.g `git push downstream downstream-0.5.0-02`)
9. Update the main branch on the fork: `git checkout main && git push`. This will ensure that the downstream main branch is in sync with the upstream one.

The command from step 8 will trigger 3 jobs, two that build the api and watcher images and one which creates a PR to the pipeline-service repo updating SHAs of the images and ref used in pulling the configuration for the results downstream fork.

## Downstream only PRs

We are using the upstream first approach, but sometimes we need a change only to the downstream fork. We follow the regular process here, but we squash all downstream commits every time we sync with upstream.

1. Fetch the downstream fork: `git fetch downstream` and find the most recent downstream branch (e.g `downstream/downstream-0.5.0-02`)
2. Create a new feature branch from the downstream branch: `git checkout <your-feature-branch> -b downstream/<downstream-X.Y.Z-NN>` (e.g. `git checkout -b add-downstream-sync-doc downstream/downstream-0.5.0-02`)
3. When your change is ready, open a PR choosing a base repository "openshift-pipeline/tektoncd-results" and base branch the latest downstream-X.Y.Z-NN branch (here downstream-0.5.0-02)
4. Follow the regular process and merge when ready
