FROM guinsoolab/annastore:base

ENV PATH=/opt/bin:$PATH

COPY ./annastore /opt/bin/annastore

ENTRYPOINT ["/usr/bin/docker-entrypoint.sh"]

VOLUME ["/data"]

CMD ["annastore"]
