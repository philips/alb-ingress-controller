[![build status](http://git.tmaws.io/kubernetes/alb-ingress/badges/master/build.svg)](http://git.tmaws.io/kubernetes/alb-ingress/commits/master) [![coverage report](http://git.tmaws.io/kubernetes/alb-ingress/badges/master/coverage.svg)](http://git.tmaws.io/kubernetes/alb-ingress/commits/master)

# ALB Ingress Controller

The ALB Ingress Controller satisfies Kubernetes [ingress resources](https://kubernetes.io/docs/user-guide/ingress) by provisioning an [Application Load Balancer](https://aws.amazon.com/elasticloadbalancing/applicationloadbalancer) and Route 53 DNS record set.

## Usage

This section details deployment of the controller and its behavior regarding ingress resources.

### Deployment and Configuration

The ALB Ingress Controller is a [Kubernetes deployment](https://kubernetes.io/docs/user-guide/deployments). Only a single instance should be run at a time. Any issues, crashes, or other rescheduling needs will be handled by Kubernetes natively. See the [alb-ingress-controller.yaml inside examples](./examples/alb-ingress-controller.yaml) for a sample deployment manifest.

**[TODO]**: Need to validate iam-policy.json mentioned below and see if it can be refined.

In order to perform operations, the controller must be able to resolve an IAM role capable of accessing and provisioning ALB and Route53 resources. There are many ways to achieve this, such as loading `AWS_ACCESS_KEY_ID`/`AWS_ACCESS_SECRET_KEY` as environment variables or using [kube2iam](https://github.com/jtblin/kube2iam). A sample IAM policy with the minimum permissions to run the controller can be found in [examples/alb-iam-policy.json](examples/iam-policy.json).

**[TODO]**: Need to verify ingress.class, mentioned below,  works OOTB with this controller. IF not, seems very valuable to implement.

The controller will see ingress events for all namespaces in your cluster. Ingress resources that do not contain [necessary annotations](#annotations) will automatically be ignored. However, you may wish to limit the scope of ingress resources this controller has visibility into. In this case, you can define an `ingress.class` annotation, set the `--watch-namespace=` argument, or both.

Setting the `kubernetes.io/ingress.class: "alb"` annotation allows for classification of ingress resources and is especially helpful when running multiple ingress controllers in the same cluster. See [Using Multiple Ingress Controllers](https://github.com/nginxinc/kubernetes-ingress/tree/master/examples/multiple-ingress-controllers#using-multiple-ingress-controllers) for more details.

Setting the `--watch-namespace` argument constrains the ALB ingress-controller's scope to a **single** namespace. Ingress events outside of the namespace specified here will not be seen by the controller. Currently you cannot specify a watch on multiple namespaces or blacklist specific namespaces. See [this Kubernetes issue](https://github.com/kubernetes/contrib/issues/847) for more details.

Once configured as needed, the controller can be deployed like any Kubernetes deployment.

```bash
$ kubectl apply -f alb-ingress-controller.yaml
```

### Ingress Behavior

Periodically, ingress update events are seen by the controller. The controller retains a list of all ingress resources it knows about, along with the current state of AWS components that satisfy them. When an update event is fired, the controller re-scans the list of ingress resources known to the cluster and determines, by comparing the list to its previously stored one, the ingresses requiring deletion, creation or modification.

An example ingress, from `example/2048/2048-ingress.yaml` is as follows.

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: "nginx-ingress"
  namespace: "2048-game"
  annotations:
    alb.ingress.kubernetes.io/scheme: internet-facing
    alb.ingress.kubernetes.io/subnets: subnet-0d2bab64,subnet-9c569ce7
    alb.ingress.kubernetes.io/security-groups: sg-ccaacaa5
    alb.ingress.kubernetes.io/tags: Environment=dev1,ProductCode=PRD999
  labels:
    app: 2048-nginx-ingress
spec:
  rules:
  - host: 2048.yourdomain.com
    http:
      paths:
      - path: /
        backend:
          serviceName: "service-2048"
          servicePort: 80
```

The host field specifies the eventual Route 53-managed domain that will route to this service. The service, service-2048, must be of type NodePort (see [examples/echoservice/echoserver-service.yaml](examples/echoservice/echoserver-service.yaml)) in order for the provisioned ALB to route to it. If no NodePort exists, the controller will not attempt to provision resources in AWS. For details on purpose of annotations seen above, see [Annotations](#annotations).

## Annotations

The following annotations, when added to an ingress resource, are respected by the ALB Ingress Controller.

```
alb.ingress.kubernetes.io/backend-protocol
alb.ingress.kubernetes.io/certificate-arn
alb.ingress.kubernetes.io/healthcheck-path
alb.ingress.kubernetes.io/port
alb.ingress.kubernetes.io/scheme
alb.ingress.kubernetes.io/security-groups
alb.ingress.kubernetes.io/subnets
alb.ingress.kubernetes.io/successCodes
alb.ingress.kubernetes.io/tags
```

The following describes each annotations use, namespaces are omitted for brevity.

- **backend-protocol**: Optional. Enables selection of protocol for ALB to use to connect to backend service. When omitted, `HTTP` is used.

- **certificate-arn**: Optional. Enables HTTPS and uses the certificate defined, based on arn, stored in your [AWS Certificate Manager](https://aws.amazon.com/certificate-manager).

- **healthcheck-path**: Optional. Defines the path ALB health checks will occur. When omitted, `/` is used.

- **port**: Optional. Defines the port the ALB is exposed. When omitted, `80` is used for HTTP and `443` is used for HTTPS.

- **scheme**: Required. Defines whether an ALB should be `internal` or `internet-facing`. See [Load balancer scheme] in the AWS documentation for more details.

- **security-groups**: Required. [Security groups](http://docs.aws.amazon.com/AmazonVPC/latest/UserGuide/VPC_SecurityGroups.html) that should be applied to the ALB instance.

- **subnets**: Required. The subnets where the ALB instance should be deployed. Must include 2 subnets, each in a different [availability zone](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/using-regions-availability-zones.html).

- **successCodes**: Optional. Defines the HTTP status code that should be expected when doing health checks against the defined `healthcheck-path`. When omitted, `200` is used.

- **Tags**: Optional. Defines [AWS Tags](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/Using_Tags.html) that should be applied to the ALB instance and Target groups.

## Building

For details on building this project, see [BUILDING.md](./BUILDING.md).
