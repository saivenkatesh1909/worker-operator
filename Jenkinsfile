@Library('jenkins-library@feature/opensource') _
dockerImagePipeline1(
  script: this,
  service: 'aveshadev/operator',
  dockerfile: 'Dockerfile',
  buildContext: '.',
  buildArguments: [PLATFORM:"amd64"],
  trigger_remote: 'yes'
)
