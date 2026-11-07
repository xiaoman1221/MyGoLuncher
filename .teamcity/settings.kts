import jetbrains.buildServer.configs.kotlin.*
import jetbrains.buildServer.configs.kotlin.buildSteps.*
import jetbrains.buildServer.configs.kotlin.triggers.*

version = "2026.2"

project {
    buildType(SecurityChecks)
    buildType(WindowsCommercialRelease)
    buildType(MacOSCommercialRelease)
    buildType(LinuxCommercialRelease)
}

// Runs on every master change (except generated release commits). Release
// builds also run the same script as a gate before compiling.
object SecurityChecks : BuildType({
    id("MyGoLuncher_SecurityChecks")
    name = "Security Checks"
    description = "Secret scan, sensitive-file check, go vet, govulncheck and npm audit."

    requirements {
        contains("teamcity.agent.jvm.os.name", "Linux")
    }

    vcs {
        root(DslContext.settingsRoot)
    }

    steps {
        script {
            name = "Security checks"
            scriptContent = "node scripts/teamcity-security.mjs"
        }
    }

    triggers {
        vcs {
            branchFilter = "+:refs/heads/master"
            triggerRules = "-:releases/**"
        }
    }
})


// All three build types are manual and use the same cross-platform Node
// release script. Each one produces an official commercial build and commits
// only its own platform directory under releases/.
object WindowsCommercialRelease : BuildType({
    id("MyGoLuncher_WindowsCommercialRelease")
    name = "Windows Commercial Release"
    description = "Official commercial Windows build: product config, UPX packing, Authenticode signing."

    artifactRules = "releases/** => releases"

    params {
        password("env.MGL_PRODUCT_API_KEY", "")
        param("env.MGL_PRODUCT_SERVER_URL", "")
        param("env.MGL_PRODUCT_NAME", "MyGoLuncher")
        param("env.MGL_PRODUCT_SLUG", "MyGo")
        param("env.MGL_PLATFORM", "windows")

        param("env.MGL_PFX_PATH", "")
        password("env.MGL_SIGN_PFX_PASSWORD", "")
        param("env.MGL_UPX_PATH", "")

        password("env.GIT_PUSH_TOKEN", "")
        param("env.GIT_PUSH_USER", "xiaoman1221")
        param("env.MGL_GIT_PUSH_URL", "https://git.1v.fit/xiaoman1221/MyGoLuncher.git")
    }

    requirements {
        contains("teamcity.agent.jvm.os.name", "Windows")
    }

    vcs {
        root(DslContext.settingsRoot)
    }

    steps {
        script {
            name = "Security gate"
            scriptContent = "node scripts/teamcity-security.mjs"
        }
        script {
            name = "Build, pack, sign and publish"
            scriptContent = "node scripts/teamcity-release.mjs --platform windows --branch \"%teamcity.build.branch%\" --build-number \"%build.number%\""
        }
    }
})

object MacOSCommercialRelease : BuildType({
    id("MyGoLuncher_MacOSCommercialRelease")
    name = "macOS Commercial Release"
    description = "Official commercial macOS build: product config, optional UPX, optional codesign."

    artifactRules = "releases/** => releases"

    params {
        password("env.MGL_PRODUCT_API_KEY", "")
        param("env.MGL_PRODUCT_SERVER_URL", "")
        param("env.MGL_PRODUCT_NAME", "MyGoLuncher")
        param("env.MGL_PRODUCT_SLUG", "MyGo")
        param("env.MGL_PLATFORM", "macos")

        // Codesign identity from `security find-identity -v -p codesigning`.
        // Leave empty to build an unsigned artifact.
        param("env.MGL_MACOS_SIGN_IDENTITY", "")
        param("env.MGL_MACOS_TIMESTAMP", "true")
        // Disabled by default: UPX on Mach-O can invalidate code signatures.
        param("env.MGL_UPX", "false")

        password("env.GIT_PUSH_TOKEN", "")
        param("env.GIT_PUSH_USER", "xiaoman1221")
        param("env.MGL_GIT_PUSH_URL", "https://git.1v.fit/xiaoman1221/MyGoLuncher.git")
    }

    requirements {
        contains("teamcity.agent.jvm.os.name", "Mac")
    }

    vcs {
        root(DslContext.settingsRoot)
    }

    steps {
        script {
            name = "Security gate"
            scriptContent = "node scripts/teamcity-security.mjs"
        }
        script {
            name = "Build and publish"
            scriptContent = "node scripts/teamcity-release.mjs --platform macos --branch \"%teamcity.build.branch%\" --build-number \"%build.number%\""
        }
    }
})

object LinuxCommercialRelease : BuildType({
    id("MyGoLuncher_LinuxCommercialRelease")
    name = "Linux Commercial Release"
    description = "Official commercial Linux build: product config, optional UPX, optional GPG checksum signature."

    artifactRules = "releases/** => releases"

    params {
        password("env.MGL_PRODUCT_API_KEY", "")
        param("env.MGL_PRODUCT_SERVER_URL", "")
        param("env.MGL_PRODUCT_NAME", "MyGoLuncher")
        param("env.MGL_PRODUCT_SLUG", "MyGo")
        param("env.MGL_PLATFORM", "linux")

        param("env.MGL_UPX", "true")
        param("env.MGL_UPX_PATH", "")
        param("env.MGL_GPG_KEY_ID", "")

        password("env.GIT_PUSH_TOKEN", "")
        param("env.GIT_PUSH_USER", "xiaoman1221")
        param("env.MGL_GIT_PUSH_URL", "https://git.1v.fit/xiaoman1221/MyGoLuncher.git")
    }

    requirements {
        contains("teamcity.agent.jvm.os.name", "Linux")
    }

    vcs {
        root(DslContext.settingsRoot)
    }

    steps {
        script {
            name = "Security gate"
            scriptContent = "node scripts/teamcity-security.mjs"
        }
        script {
            name = "Build, pack and publish"
            scriptContent = "node scripts/teamcity-release.mjs --platform linux --branch \"%teamcity.build.branch%\" --build-number \"%build.number%\""
        }
    }
})
