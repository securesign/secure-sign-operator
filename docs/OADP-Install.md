# OADP Prerequisite

To successfully install OADP an S3 bucket is required as it is where the OADP Operator will store and fetch data for both the Backup and restore procedures.

## Create S3 Bucket and IAM User
```sh
BUCKET=<your_bucket>
REGION=<your_region>

aws s3api create-bucket \
    --bucket $BUCKET \
    --region $REGION \
    --create-bucket-configuration LocationConstraint=$REGION
```

After the bucket is created we create an IAM user to allow access to the bucket.

```sh
aws iam create-user --user-name velero
```

Once the IAM account is created we will create and add a policy to the user to allow the required access to the S3 Bucket.

```sh
cat > velero-policy.json <<EOF
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "ec2:DescribeVolumes",
                "ec2:DescribeSnapshots",
                "ec2:CreateTags",
                "ec2:CreateVolume",
                "ec2:CreateSnapshot",
                "ec2:DeleteSnapshot"
            ],
            "Resource": "*"
        },
        {
            "Effect": "Allow",
            "Action": [
                "s3:GetObject",
                "s3:DeleteObject",
                "s3:PutObject",
                "s3:AbortMultipartUpload",
                "s3:ListMultipartUploadParts"
            ],
            "Resource": [
                "arn:aws:s3:::${BUCKET}/*"
            ]
        },
        {
            "Effect": "Allow",
            "Action": [
                "s3:ListBucket",
                "s3:GetBucketLocation",
                "s3:ListBucketMultipartUploads"
            ],
            "Resource": [
                "arn:aws:s3:::${BUCKET}"
            ]
        }
    ]
}
EOF

aws iam put-user-policy \
  --user-name velero \
  --policy-name velero \
  --policy-document file://velero-policy.json

```

## Credentials File

Once the velero policy is in place we can then create the required access Key using info from the following command.

```sh
aws iam create-access-key --user-name velero
```

```sh
cat << EOF > ./credentials-velero
[default]
aws_access_key_id=<AWS_ACCESS_KEY_ID>
aws_secret_access_key=<AWS_SECRET_ACCESS_KEY>
EOF
```
Once configured the key will be pushed up onto the cluster.

```sh
oc create secret generic cloud-credentials -n openshift-adp --from-file cloud=credentials-velero
```

## OADP Install
The OADP Operator can be installed from the Openshift Operator Hub.
Note: If the cluster is running on AWS you may be prompted to provide an STS Role ARN.

Once Installed we will need to specify the DataProtectionApplicationCR.
```sh
cat << EOF > ./DataProtectionApplication.yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: DataProtectionApplication
metadata:
  name: data-protection-application
  namespace: openshift-adp 
spec:
  snapshotMoveData: true
  configuration:
    velero:
      defaultPlugins:
        - openshift 
        - aws
        - csi
      resourceTimeout: 10m 
    nodeAgent: 
      enable: true 
      uploaderType: kopia 
  backupLocations:
    - name: default
      velero:
        provider: aws
        default: true
        objectStorage:
          bucket: <BucketName>
          prefix: <S3PrefixName>
        config:
          region: <BucketRegion>
          profile: "default"
        credential:
          key: cloud
          name: cloud-credentials 
  snapshotLocations: 
    - name: default
      velero:
        provider: aws
        config:
          region: <BucketRegion>
          profile: "default"
EOF
```

Once configured run the following command to apply it to the cluster.
```sh
oc apply -f DataProtectionApplication.yaml
```

## Extra Install Info
For extra info or specifics relating to the install and or configuration of the operator see the [OADP Docs](https://docs.openshift.com/container-platform/4.14/backup_and_restore/application_backup_and_restore/installing/oadp-installing-operator.html).