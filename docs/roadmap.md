# Roadmap

The following is the roadmap for Tekton Results. Timelines may change based depending on number of developers working on the project.

## Q4 2020
- [x] API defined / TEP Approved
- [x] Project repo created
- [x] CI / other repo infra set up.

## Q1 2021
- [ ] Result API v0.1.0
    - [ ] Implement [v1alpha2 Result/Record CRUD API](https://github.com/tektoncd/community/blob/master/teps/0021-results-api.md).
    - [x] Basic TaskRun/PipelineRun filtering
    - [x] Pagination
    - [x] Deprecate/remove proof-of-concept v1alpha1 API.
- [ ] Result Watcher v0.1.0
    - [ ] TaskRun/PipelineRun result uploading
- [ ] Set up release infrastructure.
- [ ] tekton.dev docs
- [ ] Goal: Tekton Results running against [Tekton Dogfooding Cluster](https://github.com/tektoncd/plumbing/blob/master/docs/dogfooding.md).

## Q2 2021
- [ ] Result API v0.2.0
    - [ ] Authentication / Authorization
    - [ ] Trigger Event filtering
    - [ ] Notification filtering
- [ ] Result Watcher v0.2.0
    - [ ] Task/PipelineRun Cleanup
    - [ ] Trigger Events
    - [ ] Notifications

## Q3 2021
- [ ] Component integrations - CLI, Dashboard

## Q4 2021
- [ ] Promote Results API to Beta (includes conformance definition)
