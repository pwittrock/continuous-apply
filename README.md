# This is not a Google project



export ACCOUNT=$(gcloud info --format='value(config.account)'); kubectl create clusterrolebinding owner-cluster-admin-binding --clusterrole cluster-admin --user $ACCOUNT

# Install the API

- kubectl apply -f config/crds/ -f config/manager/

# Create the perms and credentials for talking to the apiserver and github

- kubectl create serviceaccount firionavie
- kubectl create clusterrolebinding applier-cluster-admin --clusterrole=cluster-admin --serviceaccount=default:firionavie
- kubectl create secret generic applier-github-credentials --from-file=./applier-github-credentials


# Define a rollout

apiVersion: apply.k8s.io/v1beta1
kind: ContinuousApply
metadata:
  name: applier-name
spec:
  repo:
    owner: pwittrock
    repo: najena
  user: firionavie-canary
  targets:
  - path: config
  - path: config2
  match:
    labels:
    - "rollout"
  components:
    gitCredentials:
     key: "firionavie-canary"
     secret:
       name: "applier-github-credentials"
    serviceAccount: "firionavie"