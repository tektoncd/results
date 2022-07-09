# Roadmap

The following is the roadmap for Tekton Results. Timelines may change based
depending on number of developers working on the project.

## Q4 2020

- [x] API defined / TEP Approved
- [x] Project repo created
- [x] CI / other repo infra set up.

## Q1 2021

- [x] Result API v0.1.0
  - [x] Implement
        [v1alpha2 Result/Record CRUD API](https://github.com/tektoncd/community/blob/main/teps/0021-results-api.md).
  - [x] Basic TaskRun/PipelineRun filtering
  - [x] Pagination
  - [x] Deprecate/remove proof-of-concept v1alpha1 API.
- [x] Result Watcher v0.1.0
  - [x] TaskRun/PipelineRun result uploading
- [x] Set up release infrastructure.
- [ ] tekton.dev docs
- [x] Goal: Tekton Results running against
      [Tekton Dogfooding Cluster](https://github.com/tektoncd/plumbing/blob/main/docs/dogfooding.md).

## Q2 2021

- [x] Result API v0.2.0
  - [x] Authentication / Authorization
  - [x] Non-Pipeline Record types (e.g. Trigger events, notifications)
- [ ] Result Watcher v0.2.0
  - [x] Task/PipelineRun Cleanup
  - [ ] Trigger Events
  - [ ] Notifications
- [x] Release Automation

## Q3 2021

- [ ] Component integrations -
  - [x] CLI
  - [ ] Dashboard

## Q4 2021

- [ ] Promote Results API to Beta (includes conformance definition)
