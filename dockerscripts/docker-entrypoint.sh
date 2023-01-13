#!/bin/sh
#

# If command starts with an option, prepend annastore.
if [ "${1}" != "annastore" ]; then
    if [ -n "${1}" ]; then
        set -- annastore "$@"
    fi
fi

# su-exec to requested user, if service cannot run exec will fail.
docker_switch_user() {
    if [ -n "${ANNASTORE_USERNAME}" ] && [ -n "${ANNASTORE_GROUPNAME}" ]; then
        if [ -n "${ANNASTORE_UID}" ] && [ -n "${ANNASTORE_GID}" ]; then
            groupadd -g "$ANNASTORE_GID" "$ANNASTORE_GROUPNAME" && \
                useradd -u "$ANNASTORE_UID" -g "$ANNASTORE_GROUPNAME" "$ANNASTORE_USERNAME"
        else
            groupadd "$ANNASTORE_GROUPNAME" && \
                useradd -g "$ANNASTORE_GROUPNAME" "$ANNASTORE_USERNAME"
        fi
        exec setpriv --reuid="${ANNASTORE_USERNAME}" \
             --regid="${ANNASTORE_GROUPNAME}" --keep-groups "$@"
    else
        exec "$@"
    fi
}

## Switch to user if applicable.
docker_switch_user "$@"
