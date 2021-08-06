#!/bin/bash

# Kills current and all background child processes on exit.
trap "trap - SIGTERM && kill -- -$$" SIGINT SIGTERM EXIT

kubectl port-forward pod/mysql 8080:3306 &
export MYSQL_USER=root
export MYSQL_PASSWORD=tacocat
export MYSQL_PROTOCOL=tcp
export MYSQL_ADDR=localhost:8080
export MYSQL_DB=results

kubectl port-forward pod/postgres 8081:5432 &
export POSTGRES_USER="postgres"
export POSTGRES_PASSWORD="tacocat"
export POSTGRES_ADDR="localhost"
export POSTGRES_PORT="8081"
export POSTGRES_DB="tekton-results"

go test -tags e2e_migrate -v -count 1 .
