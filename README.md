# node-notready-controller

This is a Kubernetes controller that we developed to tackle [broken nodes](https://github.com/kubernetes-sigs/karpenter/issues/1659) in our Cluster, which we discovered to be a problem after our migration from Spot Ocean to Karpenter (version 1.0.0).
We offered this implementation as a contribution to Karpenter in [this PR](https://github.com/kubernetes-sigs/karpenter/pull/1755) which was later on closed in favour of their own,
more cloud provider centric, implementation.

It's a very simple controller that watches for the taint `node.kubernetes.io/unreachable` and if the taint has been there for more than 10 minutes it
deletes the Nodeclaim, which will lead to the termination of the node.
