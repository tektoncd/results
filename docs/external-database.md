# Using an external database

It is possible to use any external Postgres database instead of a local one.
Tekton Results gets its database configuration from a Kubernetes Secret
named `tekton-results-postgres`. Before we create secrets, we need to create
patches for removing database components.

## Create patches

By default, Tekton Results ships with a local database installation config in
the release manifest. You can remove them using patches. You may create separate
patch files or directly add them to kustomization.yaml. Here are the changes:

- Remove database StatefulSet

```yaml
# delete-database-statefulset.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: tekton-results-postgres
  namespace: tekton-pipelines
$patch: delete
```

- Remove database Service

```yaml
# delete-database-service.yaml
apiVersion: v1
kind: Service
metadata:
  name: tekton-results-postgres-service
  namespace: tekton-pipelines
$patch: delete
```

- Remove database ConfigMap

```yaml
# delete-datbase-configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-results-postgres
  namespace: tekton-pipelines
$patch: delete
```

If you want to keep the patches in different files. Create the files as directed
above and add them to `kustomization.yaml` instead of patches:

```yaml
patches:
  - path: delete-database-statefulset.yaml
  - path: delete-database-service.yaml
  - path: delete-database-configmap.yaml
```

## Modifying DB_HOST and DB_NAME

You may add patches to modify them. Here, we will utilize env/config. If you
want to securely store these variables, consider adding patches to fetch these
value from the secret and then include these when creating database secret.

Copy the [config](../config/base/env/config) and change these values.

```cfg
DB_HOST=
DB_NAME=
```

Here is the required patch if you want to use Kubernetes Secret for passing
these values as well.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: tekton-pipelines
spec:
  template:
    spec:
      containers:
        - name: api
          env:
            - name: DB_HOST
              value: <put-your-external-db-host-here>
            - name: DB_NAME
              value: <your-db-name default:tekton-results>
```

## Create Secret for storing database username and password

We will use Kubernetes Secret for storing database username and password. By
default, Tekton Results expects the Secret with name `tekton-results-postgres`
in the `tekton-pipelines` namespace and containing `POSTGRES_USER` and
`POSTGRES_PASSWORD` fields. You can use the command below to create a secret.

 ```sh
 kubectl create secret generic tekton-results-postgres \
  --namespace="tekton-pipelines" \
  --from-literal=POSTGRES_USER=<your-db-username> \
  --from-literal=POSTGRES_PASSWORD=<your-db-password>
 ```

## Binding everything together

Please follow the step 1 and 3 from the [installation docs](./install.md).
Create a folder and then add kustomization.yaml with the following content. You
can choose a different version of Tekton Results as well.

```yaml
# kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - https://storage.googleapis.com/tekton-releases/results/latest/release.yaml

configMapGenerator:
  - behavior: replace
    files:
      # path to the config file, assuming in the same directory
      - config
    name: tekton-results-api-config
    namespace: tekton-pipelines
    options:
      disableNameSuffixHash: true

patches:
  - target:
      kind: StatefulSet
      name: tekton-results-postgres
      namespace: tekton-pipelines
    patch: |-
      apiVersion: apps/v1
      kind: StatefulSet
      metadata:
        name: postgres
        namespace: tekton-pipelines
      $patch: delete
  - target:
      kind: Service
      name: tekton-results-postgres-service
      namespace: tekton-pipelines
    patch: |-
      apiVersion: v1
      kind: Service
      metadata:
        name: postgres-service
        namespace: tekton-pipelines
      $patch: delete
  - target:
      kind: ConfigMap
      name: tekton-results-postgres
      namespace: tekton-pipelines
    patch: |-
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: postgres
        namespace: tekton-pipelines
      $patch: delete
```

- Install with Kustomization

```sh
kubectl apply --kustomize ./<directory-name>/ 
```
