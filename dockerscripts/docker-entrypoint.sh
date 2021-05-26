#!/bin/sh
#

# If command starts with an option, prepend minio.
if [ "${1}" != "minio" ]; then
    if [ -n "${1}" ]; then
        set -- minio "$@"
    fi
fi

## Look for docker secrets at given absolute path or in default documented location.
docker_secrets_env_old() {
    if [ -f "$MINIO_ACCESS_KEY_FILE" ]; then
        ACCESS_KEY_FILE="$MINIO_ACCESS_KEY_FILE"
    else
        ACCESS_KEY_FILE="/run/secrets/$MINIO_ACCESS_KEY_FILE"
    fi
    if [ -f "$MINIO_SECRET_KEY_FILE" ]; then
        SECRET_KEY_FILE="$MINIO_SECRET_KEY_FILE"
    else
        SECRET_KEY_FILE="/run/secrets/$MINIO_SECRET_KEY_FILE"
    fi

    if [ -f "$ACCESS_KEY_FILE" ] && [ -f "$SECRET_KEY_FILE" ]; then
        if [ -f "$ACCESS_KEY_FILE" ]; then
            MINIO_ACCESS_KEY="$(cat "$ACCESS_KEY_FILE")"
            export MINIO_ACCESS_KEY
        fi
        if [ -f "$SECRET_KEY_FILE" ]; then
            MINIO_SECRET_KEY="$(cat "$SECRET_KEY_FILE")"
            export MINIO_SECRET_KEY
        fi
    fi
}

docker_secrets_env() {
    if [ -f "$MINIO_ROOT_USER_FILE" ]; then
        ROOT_USER_FILE="$MINIO_ROOT_USER_FILE"
    else
        ROOT_USER_FILE="/run/secrets/$MINIO_ROOT_USER_FILE"
    fi
    if [ -f "$MINIO_ROOT_PASSWORD_FILE" ]; then
        SECRET_KEY_FILE="$MINIO_ROOT_PASSWORD_FILE"
    else
        SECRET_KEY_FILE="/run/secrets/$MINIO_ROOT_PASSWORD_FILE"
    fi

    if [ -f "$ROOT_USER_FILE" ] && [ -f "$SECRET_KEY_FILE" ]; then
        if [ -f "$ROOT_USER_FILE" ]; then
            MINIO_ROOT_USER="$(cat "$ROOT_USER_FILE")"
            export MINIO_ROOT_USER
        fi
        if [ -f "$SECRET_KEY_FILE" ]; then
            MINIO_ROOT_PASSWORD="$(cat "$SECRET_KEY_FILE")"
            export MINIO_ROOT_PASSWORD
        fi
    fi
}

## Set KMS_SECRET_KEY from docker secrets if provided
docker_kms_secret_encryption_env() {
    if [ -f "$MINIO_KMS_SECRET_KEY_FILE" ]; then
        KMS_SECRET_KEY_FILE="$MINIO_KMS_SECRET_KEY_FILE"
    else
        KMS_SECRET_KEY_FILE="/run/secrets/$MINIO_KMS_SECRET_KEY_FILE"
    fi

    if [ -f "$KMS_SECRET_KEY_FILE" ]; then
        MINIO_KMS_SECRET_KEY="$(cat "$KMS_SECRET_KEY_FILE")"
        export MINIO_KMS_SECRET_KEY
    fi
}

## Legacy
## Set KMS_MASTER_KEY from docker secrets if provided
docker_kms_master_encryption_env() {
    KMS_MASTER_KEY_FILE="/run/secrets/$MINIO_KMS_MASTER_KEY_FILE"

    if [ -f "$KMS_MASTER_KEY_FILE" ]; then
        MINIO_KMS_MASTER_KEY="$(cat "$KMS_MASTER_KEY_FILE")"
        export MINIO_KMS_MASTER_KEY
    fi
}

# su-exec to requested user, if service cannot run exec will fail.
docker_switch_user() {
    if [ ! -z "${MINIO_USERNAME}" ] && [ ! -z "${MINIO_GROUPNAME}" ]; then
        if [ ! -z "${MINIO_UID}" ] && [ ! -z "${MINIO_GID}" ]; then
            groupadd -g "$MINIO_GID" "$MINIO_GROUPNAME" && \
                useradd -u "$MINIO_UID" -g "$MINIO_GROUPNAME" "$MINIO_USERNAME"
        else
            groupadd "$MINIO_GROUPNAME" && \
                useradd -g "$MINIO_GROUPNAME" "$MINIO_USERNAME"
        fi
        exec setpriv --reuid="${MINIO_USERNAME}" --regid="${MINIO_GROUPNAME}" --keep-groups "$@"
    else
        exec "$@"
    fi
}

## Set access env from secrets if necessary. Legacy
docker_secrets_env_old

## Set access env from secrets if necessary. Override
docker_secrets_env

## Set sse encryption from secrets if necessary. Legacy
docker_kms_master_encryption_env

## Set kms encryption from secrets if necessary. Override
docker_kms_secret_encryption_env

## Switch to user if applicable.
docker_switch_user "$@"
