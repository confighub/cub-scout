#!/bin/bash
#kube-gen.sh resource-type args
res="${1}"
shift 1
if [[ "${res}" != namespace ]] ; then
  namespace="--namespace=confighubplaceholder"
else
  namespace=""
fi
kubectl create "${res}" -o yaml --dry-run=client "${@}" ${namespace} |\
egrep -v "creationTimestamp|status"
