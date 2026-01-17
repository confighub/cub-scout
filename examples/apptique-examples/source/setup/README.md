# Demo setup

Prerequisites:

- ConfigHub account ([signup](https://auth.confighub.com/sign-up))
- cub ([install](https://docs.confighub.com/get-started/setup/#install-the-cli))
- kubectl
- kind
- flux
- helm (if using setup-helm.sh)

Setup:

```
git clone https://github.com/confighub-kubecon-2025/setup
git clone https://github.com/confighub-kubecon-2025/appchat
git clone https://github.com/confighub-kubecon-2025/appvote
git clone https://github.com/confighub-kubecon-2025/apptique
setup/setup-clusters.sh
# Wait for nginx to start
sleep 30
setup/setup-cub.sh
```

Add the following hosts to /etc/hosts:

```
127.0.0.1 dev.appchat.cubby.bz
127.0.0.1 www.appchat.cubby.bz
127.0.0.1 dev-vote.appvote.cubby.bz
127.0.0.1 dev-results.appvote.cubby.bz
127.0.0.1 www.appvote.cubby.bz
127.0.0.1 results.appvote.cubby.bz
127.0.0.1 dev.apptique.cubby.bz
127.0.0.1 www.apptique.cubby.bz
```

You should then be able to access the instances at:

- Dev
  - http://dev.appchat.cubby.bz:11080/
  - http://dev-vote.appvote.cubby.bz:11080/
  - http://dev-results.appvote.cubby.bz:11080/
  - http://dev.apptique.cubby.bz:11080/
- Prod
  - http://www.appchat.cubby.bz:12080/
  - http://www.appvote.cubby.bz:12080/
  - http://results.appvote.cubby.bz:12080/
  - http://www.apptique.cubby.bz:12080/

Some operations you can try on these applications in ConfigHub:

- Navigate from a unit to the UI: `cub k8s source --kubeconfig prod.kubeconfig Deployment backend -n appchat`
- Change resources: `cub function invoke --space appchat-prod --unit backend set-container-resources backend floor 100m 200Mi 2`
- Change an environment variable: `cub function invoke --space appchat-prod --unit backend set-env-var backend EXPERIMENTAL_FEATURE false`
- Set security context to best-practice values: `cub function invoke --space appchat-prod --unit backend -- set-pod-defaults –security-context=true`
- Break glass and then refresh the config in ConfigHub:
  ```
  kubectl --kubeconfig prod.kubeconfig edit deploy -n apptique paymentservice
  cub unit refresh --space apptique-prod paymentservice
  cub k8s source --kubeconfig prod.kubeconfig Deployment paymentservice -n apptique
  ```
- Undo that change: `cub unit update --patch --restore -1 --space apptique-prod paymentservice`

Alternative ways to run the applications:

- To import helm charts, use setup-cubhelm.sh instead.
- To run with Flux, use setup-flux.sh instead.
- To run with just Helm, use setup-helm.sh instead.
