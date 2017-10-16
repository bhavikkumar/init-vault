FROM scratch
ADD ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ADD init-vault /init-vault
ENTRYPOINT ["/init-vault"]
