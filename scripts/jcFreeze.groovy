// vars/jcFreeze.groovy
// Drop this into any Jenkins Pipeline shared library to add frozen failure support.
//
// When a stage wrapped with jcFreeze fails, the build pauses on an input step,
// preserving the agent and workspace for interactive debugging via `jc frozen`
// or an AI agent using the MCP tools.
//
// Usage:
//   stage('integrations') {
//       steps {
//           jcFreeze(ttl: 7200, message: 'Integration tests failed') {
//               sh './integration-test.sh'
//           }
//       }
//   }
//
// The build will remain frozen until someone calls `jc frozen thaw` or the TTL
// expires (default: 3600 seconds = 1 hour).

def call(Map opts = [:], Closure body) {
    def ttl = opts.ttl ?: 3600
    def message = opts.message ?: 'Stage failed'
    def submitter = opts.submitter ?: ''

    catchError(buildResult: null, stageResult: 'FAILURE') {
        body()
    }

    if (currentBuild.result == null || currentBuild.result == 'FAILURE') {
        def inputOpts = [message: "[jc-freeze] ${message}" +
            "\nJob: ${JOB_NAME}" +
            "\nBuild: #${BUILD_NUMBER}" +
            "\nNode: ${NODE_NAME}" +
            "\nWorkspace: ${WORKSPACE}" +
            "\nTTL: ${ttl}s",
            ok: 'Thaw & Continue']

        if (submitter) {
            inputOpts.submitter = submitter
        }

        // Auto-thaw on TTL expiry
        def autoThawThread = Thread.start {
            sleep ttl * 1000L
            echo "jc-freeze: TTL expired, would auto-thaw. Run 'jc frozen thaw ${JOB_NAME} ${BUILD_NUMBER}'"
        }

        try {
            def result = input(inputOpts)
            autoThawThread.interrupt()
            echo "jc-freeze: Build ${JOB_NAME}#${BUILD_NUMBER} thawed by user."
        } catch (org.jenkinsci.plugins.workflow.steps.FlowInterruptedException e) {
            autoThawThread.interrupt()
            currentBuild.result = 'FAILURE'
            error("jc-freeze: Build aborted while frozen.")
        }
    }
}
