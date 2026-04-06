pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "agently-android"

include(":app")
include(":forge-sdk")
project(":forge-sdk").projectDir = file("../../forge/android/sdk")
include(":agently-core-sdk")
project(":agently-core-sdk").projectDir = file("../../agently-core/sdk/android")
