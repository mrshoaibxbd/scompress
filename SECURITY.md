# Security Policy

## Supported versions

Security fixes are developed for the latest supported release. Older releases may not receive fixes; update to the latest release before reporting an issue.

## Reporting a vulnerability

Please do not disclose exploitable vulnerabilities in a public issue.

Use GitHub's **Report a vulnerability** feature (private vulnerability reporting / security advisories) if it is enabled for this repository. If it is not enabled, contact the maintainer privately through the contact method listed on the maintainer's GitHub profile.

Include:

- A description of the issue and its impact.
- Affected version, distribution, architecture, and desktop session.
- Reproduction steps or a minimal proof of concept that does not contain private data.
- Any suggested mitigation.

Do not send real personal photographs or files containing private EXIF/GPS metadata.

## Image-processing safety

Treat image files from untrusted sources as untrusted input. Keep backups before enabling in-place replacement. The replacement workflow uses staging and recovery links, but no application can guarantee recovery after every filesystem, hardware, or power failure.
