<div align="right">
    <img src="https://raw.githubusercontent.com/GuinsooLab/glab/main/src/images/guinsoolab-badge.png" height="60" alt="badge">
    <br />
</div>
<div align="center">
    <img src="https://raw.githubusercontent.com/GuinsooLab/glab/main/src/images/guinsoolab-annastore.svg" alt="logo" height="100" />
    <br />
    <br />
</div>

# AnnaStore

> High performance OSS storage platform.

AnnaStore supports the widest range of use cases across the largest number of environments. Cloud native
since inception, AnnaStore’s software-defined suite runs seamlessly in the public cloud, private cloud and at the
edge - making it a leader in the hybrid cloud. With industry leading performance and scalability, AnnaStore can
deliver a range of use cases from AI/ML, analytics, backup/restore and modern web and mobile apps.

## Main Feature

- Active-active replication
- Encryption
- Bucket & object immutability
- AWS S3 compatibility
- Data life cycle management & tiering
- Scalability
- Identity & access management

## Quickstart

```bash
docker pull guinsoolab/annastore
docker run \
  -p 9000:9000 \
  -p 9001:9001 \
  guinsoolab/annastore:${LATEST_TAG} \
  server /Users/admin \
  --console-address ":9001"
```

## Documentation

- [Welcome to AnnaStore!](https://ciusji.gitbook.io/annastore/)
- Install and Deploy
  - [Deploy with Docker](https://ciusji.gitbook.io/annastore/install-and-deploy/deploy-with-docker)
  - [Deploy with Binary package](https://ciusji.gitbook.io/annastore/install-and-deploy/deploy-with-binary-pakcage)
- Core Concepts
  - [Erasure Coding](https://ciusji.gitbook.io/annastore/core-concepts/erasure-coding)
- Monitoring and Alerts
  - [Metrics and Alerts](https://ciusji.gitbook.io/annastore/monitoring-and-alerts/metrics-and-alerts)
  - [Health-check API](https://ciusji.gitbook.io/annastore/monitoring-and-alerts/health-check-api)
- Identity Management
  - [LDAP](https://ciusji.gitbook.io/annastore/identity-management/ldap)
  - [OpenID](https://ciusji.gitbook.io/annastore/identity-management/openid)
- Reference
  - [AnnaStore Client](https://ciusji.gitbook.io/annastore/reference/annastore-client)
  - [AnnaStore Server](https://ciusji.gitbook.io/annastore/reference/annastore-server)
  - [Integrations](https://ciusji.gitbook.io/annastore/reference/integrations)
- Appendix
  - [FAQ](https://ciusji.gitbook.io/annastore/appendix/faq)

## Upgrading AnnaStore

Upgrades require zero downtime in AnnaStore, all upgrades are non-disruptive, all transactions on AnnaStore are atomic. So upgrading all the servers simultaneously is the recommended way to upgrade AnnaStore.

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
- [The Annastore documentation website](https://ciusji.gitbook.io/annastore/)
- [The Ecosystem of Annastore 🌈](https://ciusji.gitbook.io/guinsoolab/)

## Contribute to AnnaStore Project

Please follow AnnaStore [Contributor's Guide](https://github.com/GuinsooLab/annastore/blob/main/CONTRIBUTING.md)

## License

Use of AnnaStore is governed by the GNU AGPLv3 license that can be found in the [LICENSE](./LICENSE) file.

<img src="https://raw.githubusercontent.com/GuinsooLab/glab/main/src/images/guinsoolab-group.svg" width="120" alt="license" />

