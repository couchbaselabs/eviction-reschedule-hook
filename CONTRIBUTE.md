# Contributing to Couchbase Reschedule Hook

Couchbase welcomes anyone that wants to help out with the Reschedule Hook project in any way! Before contributing, please review the following guidelines. Furthermore, like the code base, these guidelines will continue to evolve and we are open to suggestions on how they can be improved.

## Introduction

The Couchbase Reschedule Hook is an open source project designed to help with the graceful handling of eviction requests for operator-managed Kubernetes pods. We welcome contributions from the community to help improve and expand the project's capabilities. See the project's [README](README.md) for an overview of how it works.

## How to Contribute

### Reporting Bugs

If you've found a bug, please create an issue in our GitHub repository with the following information:

- A clear, descriptive title
- Steps to reproduce the issue
- Expected behavior
- Actual behavior
- Environment details (Kubernetes version, OS, etc.)
- Any relevant logs or error messages
- Screenshots if applicable

### Requesting Features

We welcome feature requests! When submitting a feature request:

- Use the "Feature Request" issue template
- Clearly describe the feature and its benefits
- Explain why this feature would be useful to most users
- If possible, provide examples of how the feature would work

### Submitting Pull Requests

1. Fork the repository
2. Create a new branch for your changes
3. Make your changes following our coding guidelines
4. Add or update tests as needed
5. Ensure both the unit tests and e2e tests pass
6. Submit a pull request with a clear description of the changes

Pull requests must be approved by one of the project codeowners. They must also pass the required [workflow](.github/workflows/pull-ci.yml). To run the workflow before creating a pull request, use:
```bash
make act-workflow
```

## Development Setup

The following tools are required to work with the codebase:

- [git](https://git-scm.com/)
- [go](https://go.dev/)
- [docker](https://www.docker.com/)
- [GNU Make](https://www.gnu.org/software/make/manual/make.html)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)

Recommended development tools:
- [kind](https://kubernetes.io/docs/tasks/tools/) - For running K8s locally
- [act](https://github.com/nektos/act) - For running GitHub Actions locally
- [golangci-lint](https://golangci-lint.run/) - For code linting

Please refer to the [Deployment](#deployment) section in our [README.md](README.md) for set up instructions.

## Testing Contributions

Tests are crucial for maintaining code quality and preventing regressions. All contributions must include appropriate unit and e2e tests. Adding e2e tests should be fairly straightforward using the existing framework. Keep in mind these should be testing the functionality of the reschedule hook and not how operators react to pods marked for rescheduling. That behaviour should be simulated as part of the test.

To run unit tests:
```bash
make test-unit
```
To run e2e tests:
```bash
make test-e2e
```
To run all tests:
```bash
make test
```
The e2e tests will require a running K8s cluster. If this is not available, consider running the test workflow using [act](#submitting-pull-requests).

### Test Guidelines

- Follow the existing testing framework and style
- Write unit tests using table-driven testing
- Include e2e tests for complex features
- Ensure all tests pass before submitting a PR
- Maintain or improve the project's test coverage

## Coding Guidelines

This project is a Kubernetes validating admission webhook. As such, all contributions should follow idiomatic Go conventions and align with best practices for writing webhook handlers. This includes implementing robust error handling and efficient request processing. They must also handle concurrent eviction requests safely. Code should be clean, readable, well documented and maintainable. Special attention should be paid to concurrency safety and performance optimization.

### Code formatting

The project uses the default golangci-lint formatting for code linting. Check for issues using:
```bash
make lint
```
This will run as part of the pull request workflow and needs to have 0 issues before merging is allowed.

### Style Guide

- Use meaningful variable and function names
- Add comments for complex logic
- Keep functions focused and concise
- Follow the project's existing code style

### Commit Messages

Keep commit messages clear and concise. The first line should be a brief summary of the changes, followed by a blank line and then a more detailed description if needed.

Good examples:
```
Add support for custom tracking resource types

Added the ability to configure which resource type is used to track rescheduled pods.
This allows for more flexibility in how pods are tracked across the cluster.
```

```
Fix webhook validation for non-Couchbase pods

The webhook was incorrectly validating all pods in the cluster. Now it only
validates pods that match the configured label selector.
```

## License

This project is licensed under the Apache License 2.0. By contributing to this project, you agree that your contributions will be licensed under the same license. See the [LICENSE](LICENSE) file for details.

## Getting Help

If you need help or have questions about contributing:

- Open an issue for general questions
- Review existing documentation in the [README.md](README.md)

Thank you for contributing to the Couchbase Reschedule Hook project!
