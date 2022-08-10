<div align="right">
    <img src="./public/guinsoolab-badge.png" width="60" alt="badge">
    <br />
</div>
<div align="center">
    <img src="./public/annaStore.svg" width="120" alt="logo" />
    <br/>
    <small>a high performance object storage system | powered by <a href="https://min.io/">minio</a></small>
</div>

# AnnaStore

[Overview](https://ciusji.gitbook.io/guinsoolab/products/data-storage/overview) | 
[Quickstart](https://ciusji.gitbook.io/guinsoolab/products/data-storage/annastore/quickstart) | 
[Deployment & Management](https://ciusji.gitbook.io/guinsoolab/products/data-storage/annastore/deployment-and-management) |
[Integrations](https://ciusji.gitbook.io/guinsoolab/products/data-storage/integrations) | 
[Monitoring](https://ciusji.gitbook.io/guinsoolab/products/data-storage/annastore/monitoring) | 
[Ecosystem](https://guinsoolab.github.io/glab) |


For more information, please referrer [GuinsooLab](https://guinsoolab.github.io/glab/).

## Introduction

AnnaStore supports the widest range of use cases across the largest number of environments. Cloud native
since inception, AnnaStore’s software-defined suite runs seamlessly in the public cloud, private cloud and at the
edge - making it a leader in the hybrid cloud. With industry leading performance and scalability, AnnaStore can
deliver a range of use cases from AI/ML, analytics, backup/restore and modern web and mobile apps.

## Main Feature

- Hybrid cloud
- Born cloud native
- AnnaStore is pioneering high performance object storage
- Built on the principles of web scale.
- The defacto standard for Amazon S3 compatibility 
- Simply powerful

## Upgrading AnnaStore

Upgrades require zero downtime in MinIO, all upgrades are non-disruptive, all transactions on Annastore are atomic. So upgrading all the servers simultaneously is the recommended way to upgrade AnnaStore.

- For deployments that installed the AnnaStore server binary by hand, use [`mc admin update`](https://docs.min.io/minio/baremetal/reference/minio-mc-admin/mc-admin-update.html)

```sh
mc admin update <annastore or minio alias, e.g., mystore>
```

## Explore Further

- [MinIO Erasure Code QuickStart Guide](https://docs.min.io/docs/minio-erasure-code-quickstart-guide)
- [Use `mc` with AnnaStore/MinIO Server](https://docs.min.io/docs/minio-client-quickstart-guide)
- [Use `aws-cli` with AnnaStore/MinIO Server](https://docs.min.io/docs/aws-cli-with-minio)
- [Use `s3cmd` with AnnaStore/MinIO Server](https://docs.min.io/docs/s3cmd-with-minio)
- [Use `minio-go` SDK with AnnaStore/MinIO Server](https://docs.min.io/docs/golang-client-quickstart-guide)
- [The Annastore documentation website](https://ciusji.gitbook.io/guinsoolab/products/data-storage/annastore/overview)
- [The Ecosystem of Annastore](https://ciusji.gitbook.io/guinsoolab/)

## Contribute to Annastore Project

Please follow AnnaStore [Contributor's Guide](https://github.com/GuinsooLab/annastore/blob/main/CONTRIBUTING.md)

## Documentation

[AnnaStore Guide](https://ciusji.gitbook.io/guinsoolab/products/data-storage/annastore/overview).

## License

Use of AnnaStore is governed by the GNU AGPLv3 license that can be found in the [LICENSE](./LICENSE) file.
