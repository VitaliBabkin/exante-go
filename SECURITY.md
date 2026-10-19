# Security

Report a vulnerability privately through GitHub's "Report a vulnerability" (Security tab of the repository), not in a
public issue. Include the version, a description and, if possible, a reproduction. Expect an answer within a week.

The library handles API credentials (client id, application id, shared key) and the JWT derived from them. The library
writes no logs, and `Credentials` redacts the shared key when formatted or logged; keep the credentials out of your
own logs as well.
