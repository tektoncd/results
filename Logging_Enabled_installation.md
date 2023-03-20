# Installing Results with PVC Logging in Openshift

## Configure Postgres and TLS

``` !bash
// Create tekton-pipelines ns
oc new-project tekton-pipelines

// Generate Postgres Secret
oc create secret generic tekton-results-postgres --namespace="tekton-pipelines" --from-literal=POSTGRES_USER=postgres --from-literal=POSTGRES_PASSWORD=$(openssl rand -base64 20)
openssl req -x509 \
-newkey rsa:4096 \
-keyout key.pem \
-out cert.pem \
-days 365 \
-nodes \
-subj "/CN=tekton-results-api-service.tekton-pipelines.svc.cluster.local" \
-addext "subjectAltName = DNS:tekton-results-api-service.tekton-pipelines.svc.cluster.local"

# Create new TLS Secret from cert.
kubectl create secret tls -n tekton-pipelines tekton-results-tls \
--cert=cert.pem \
--key=key.pem
```


## Install
Install the latest release.yaml
``` !bash
oc apply -f https://storage.googleapis.com/tekton-releases/results/latest/release.yaml
```

## Configure PVC

Create PVC

```!bash
cat <<EOF > pvc.yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: tekton-logs
  namespace: tekton-pipelines
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
EOF
// Apply the above PVC
oc apply -f pvc.yaml
```

### Configure Results API server to use PVC

```
cat <<EOF > deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  annotations:
    deployment.kubernetes.io/revision: "1"
  labels:
    app.kubernetes.io/name: tekton-results-api
    app.kubernetes.io/part-of: tekton-results
    app.kubernetes.io/version: 9f84a1f
  name: tekton-results-api
  namespace: tekton-pipelines
spec:
  progressDeadlineSeconds: 600
  replicas: 1
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app.kubernetes.io/name: tekton-results-api
      app.kubernetes.io/version: 9f84a1f
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 25%
    type: RollingUpdate
  template:
    metadata:
      annotations:
        cluster-autoscaler.kubernetes.io/safe-to-evict: "false"
      creationTimestamp: null
      labels:
        app.kubernetes.io/name: tekton-results-api
        app.kubernetes.io/version: 9f84a1f
    spec:
      containers:
      - env:
        - name: DB_HOST
          value: tekton-results-postgres-service.tekton-pipelines.svc.cluster.local
        - name: DB_USER
          valueFrom:
            secretKeyRef:
              key: POSTGRES_USER
              name: tekton-results-postgres
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              key: POSTGRES_PASSWORD
              name: tekton-results-postgres
        - name: DB_NAME
          value: tekton-results
        image: gcr.io/tekton-releases/github.com/tektoncd/results/cmd/api:9f84a1f@sha256:606816e51ebecb58fccc28f5a95699255ed8742470df673294ce25f69ffc451c
        imagePullPolicy: IfNotPresent
        name: api
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            add:
            - NET_BIND_SERVICE
            drop:
            - ALL
          runAsNonRoot: true
          seccompProfile:
            type: RuntimeDefault
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
        volumeMounts:
        - mountPath: /etc/tekton/results
          name: config
          readOnly: true
        - mountPath: /etc/tls
          name: tls
          readOnly: true
        - name: tekton-logs
          mountPath: "/logs"
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      schedulerName: default-scheduler
      serviceAccount: tekton-results-api
      serviceAccountName: tekton-results-api
      terminationGracePeriodSeconds: 30
      volumes:
      - configMap:
          defaultMode: 420
          name: tekton-results-api-config
        name: config
      - name: tls
        secret:
          defaultMode: 420
          secretName: tekton-results-tls
      - name: tekton-logs
        persistentVolumeClaim:
            claimName: tekton-logs
EOF
oc apply -f deployment.yaml
```

## Change the ConfigMap of Results API

``` !bash
cat <<EOF > cm.yaml
apiVersion: v1
data:
  config: |-
    DB_USER=
    DB_PASSWORD=
    DB_HOST=
    DB_PORT=5432
    DB_NAME=
    DB_SSLMODE=disable
    DB_ENABLE_AUTO_MIGRATION=true
    GRPC_PORT=50051
    REST_PORT=8080
    PROMETHEUS_PORT=9090
    TLS_HOSTNAME_OVERRIDE=
    TLS_PATH=/etc/tls
    NO_AUTH=false
    LOG_LEVEL=info
    LOGS_API=true
    LOGS_TYPE=File
    LOGS_BUFFER_SIZE=32768
    LOGS_PATH=/logs
    S3_BUCKET_NAME=
    S3_ENDPOINT=
    S3_HOSTNAME_IMMUTABLE=false
    S3_REGION=
    S3_ACCESS_KEY_ID=
    S3_SECRET_ACCESS_KEY=
    S3_MULTI_PART_SIZE=5242880
kind: ConfigMap
metadata:
  labels:
    app.kubernetes.io/part-of: tekton-results
    app.kubernetes.io/version: 9f84a1f
  name: tekton-results-api-config
  namespace: tekton-pipelines
EOF
oc apply -f cm.yaml
```

## Delete the results pod

``` !bash
oc delete pod --all -n tekton-pipelines
```


This should bring up the results with PVC configured as Logging storage.

