#!/bin/bash

# Publish Sunrise docker images to hub.docker.com

function containerName() {
  if [ "$1" == "alldbs" ]; then
    # For alldbs, container name is simply sunrise.
    local name="sunrise"
  else
    # Otherwise, sunrise-$dbtag.
    local name="sunrise-${dbtag}"
  fi
  echo $name
}

for line in $@; do
  eval "$line"
done

tag=${tag#?}

if [ -z "$tag" ]; then
    echo "Must provide tag as 'tag=v1.2.3' or 'v1.2.3-abc0'"
    exit 1
fi

# Convert tag into a version
ver=( ${tag//./ } )

if [[ ${ver[2]} != *"-"* ]]; then
  FULLRELEASE=1
fi

if [ "$db" ]; then
  dbtags=( "$db" )
else
  dbtags=( mysql postgres mongodb rethinkdb alldbs )
fi

# Read dockerhub login/password from a separate file
source .dockerhub

# Login to docker hub
docker login -u $user -p $pass

# Deploy images for various DB backends
for dbtag in "${dbtags[@]}"
do
  name="$(containerName $dbtag)"
  # Deploy tagged image
  if [ -n "$FULLRELEASE" ]; then
    docker push sunrise/${name}:latest
    docker push sunrise/${name}:"${ver[0]}.${ver[1]}"
  fi
  docker push sunrise/${name}:"${ver[0]}.${ver[1]}.${ver[2]}"
done

if [ "$db" ]; then
  exit 0
fi

# Deploy chatbot images
if [ -n "$FULLRELEASE" ]; then
  docker push sunrise/chatbot:latest
  docker push sunrise/chatbot:"${ver[0]}.${ver[1]}"
fi
docker push sunrise/chatbot:"${ver[0]}.${ver[1]}.${ver[2]}"

# Deploy exporter images
if [ -n "$FULLRELEASE" ]; then
  docker push sunrise/exporter:latest
  docker push sunrise/exporter:"${ver[0]}.${ver[1]}"
fi
docker push sunrise/exporter:"${ver[0]}.${ver[1]}.${ver[2]}"

docker logout
