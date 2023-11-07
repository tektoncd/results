# Database SSL mode with a custom CA certificate

It is possible to encrypt the traffic between the API pod and the database
using custom CA certificates.

## Configuring the database

The process of creating the CA certificate and server sertificate to be used by
the database is our of scope for this document, but one can use the process
described [here](https://www.postgresql.org/docs/current/ssl-tcp.html) as a
reference. The database can enforce certificate validation, but this is not
something Tekton Results control, so we leave it out of scope as well. Instead,
we'll focus on the client side.

## Configuring Tekton Results SSL mode

The are two configuration options in the [config](../config/base/env/config)
file controlling the client behavior:

```cfg
DB_SSLMODE=
DB_SSLROOTCERT=
```

DB_SSLMODE sets the SSL. We can use that option to disable the SSL validation
and encryption, enforce it on multiple levels or allow it without enforcing it.
The supported options are documented [here](https://www.postgresql.org/docs/current/libpq-ssl.html#LIBPQ-SSL-PROTECTION).

In case we enforce the SSL validation and encryption, the custom CA certificate
used to sign the server certificate must be made available on the Tekton Results
API pod and DB_SSLROOTCERT must be set to match the PATH to that certificate.
The most common way to do that is to store the CA certificate as a configMap and
mount that configMap as a volume. For example, if the CA certificate is stored in
a file named root.crt in the working folder, the following command can be used
to create configMap named db-root-crt:

```sh
kubectl create configmap db-root-crt -n tekton-pipelines --from-file ca.crt=./root.crt
```

To make the CA certificate available in the container, add the volume and the
volumeMount to the Tekton Results API deployment:

```yaml
          volumeMounts:
            - name: postgredb-tls-ca
              mountPath: "/etc/tls/db"
              readOnly: true
```

```yaml
      volumes:
        - name: postgredb-tls-ca
          configMap:
            name: db-root-crt
```

With that configuration, the CA certificate will be available in the
container as "/etc/tls/db/ca.crt", so you set DB_SSLROOTCERT to that value.

The Tekton Results API tries to comment to the database on start, so you'll
immediately find in the logs if something is not working properly.
