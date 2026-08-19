# Build and deploy the Android app

Android build and deployment are managed by the single Endly workflow in [`deployment/mobile/android/run.yaml`](../deployment/mobile/android/run.yaml). Do not duplicate its Gradle or ADB steps in another workflow.

Every remote build must receive an explicit workspace endpoint. The examples use the intentionally non-operational placeholder `https://workspace.example.com/`; replace it at execution time with an approved endpoint. Never commit a private workspace hostname, credentials, cookies, or encrypted-secret references.

## Prerequisites

- macOS with JDK 17
- Android SDK and platform tools
- Endly installed and available on `PATH`
- An Android device or emulator visible to `adb`
- For local development, an Agently server and workspace available on the Mac

## Android SDK source dependencies

The Android app is self-contained through pinned Git submodules:

- `android/deps/forge`
- `android/deps/agently-core`

Clone Agently with its pinned dependencies:

```bash
git clone --recurse-submodules <agently-git-url>
```

For an existing checkout, initialize or restore them with:

```bash
git submodule update --init --recursive
```

The committed submodule pointers are the Android equivalent of versions in
`go.mod`: a clean checkout builds the same SDK revisions without sibling
repositories. Endly initializes the pinned sources before building.

For active multi-repository development, explicitly opt into sibling working
trees:

```bash
export AGENTLY_ANDROID_USE_SIBLING_SOURCES=true
```

The default remains the pinned standalone mode.

### Maven artifacts from Git releases

A Git repository URL is not itself a Maven repository. If binary SDK
dependencies are preferred later, publish Android AARs from immutable Git tags
to GitHub Packages or another Maven-compatible registry, then depend on their
`group:artifact:version` coordinates. Until that publishing pipeline exists,
the pinned submodules provide reproducible source builds without registry
credentials or separately cloned sibling repositories.

Run the workflow from its directory:

```bash
cd <agently-repository>/deployment/mobile/android
```

## 1. Select a device

List connected targets:

```bash
export ANDROID_HOME='<android-sdk-path>'
"$ANDROID_HOME/platform-tools/adb" devices -l
```

When more than one target is connected, select one explicitly:

```bash
export DEVICE_SERIAL='R3CX50LW1FM'
```

Without `DEVICE_SERIAL`, the workflow retains the established behavior of preferring a physical device and then an emulator.

## 2. Build and deploy to an approved remote workspace

Supply the endpoint explicitly and run the canonical deployment task:

```bash
export AGENTLY_ANDROID_BASE_URL='https://workspace.example.com/'
endly -t=deploy
```

The example hostname is deliberately dummy. The task rejects a missing endpoint and rejects non-HTTPS remote endpoints. It then:

1. Selects and validates the Android target.
2. Runs Android unit tests.
3. Builds the debug APK with the supplied endpoint embedded.
4. Installs it with `adb install -r`.
5. Restarts and launches Agently.

`install -r` preserves compatible app data and the current authenticated session. The workflow does not clear data unless an operator explicitly requests the legacy `CLEAR_DATA=true` behavior.

## 3. Start Agently with a local workspace

In another terminal, build the server and start it with an operator-supplied workspace path:

```bash
cd <agently-repository>
export AGENTLY_WORKSPACE='/absolute/path/to/approved/workspace'
go build -o ./bin/agently ./agently
./bin/agently serve -w="$AGENTLY_WORKSPACE"
```

The default local listener is port `8080`. Keep the process running while testing the Android app. The workspace path is intentionally not hardcoded in this public repository.

## 4. Proxy Android localhost to the Mac

On a phone, `127.0.0.1` normally refers to the phone itself. ADB reverse port forwarding changes the route for the selected USB/debug device:

```text
Android app -> 127.0.0.1:8080 -> ADB reverse tunnel -> Mac 127.0.0.1:8080 -> local Agently
```

Run the local deployment task:

```bash
export LOCAL_AGENTLY_PORT=8080
endly -t=deployLocal
```

`deployLocal` performs these steps:

1. Verifies the local Agently `/healthz` endpoint.
2. Runs `adb reverse tcp:8080 tcp:8080` for the selected device.
3. Builds Android with the explicit endpoint `http://127.0.0.1:8080/`.
4. Runs unit tests, installs in place, and launches the app.

The reverse tunnel does not open the Mac port to the LAN or Internet. It is scoped to the selected ADB device. Android permits cleartext for `localhost` and `127.0.0.1`; remote workspace deployment still requires HTTPS.

Inspect active mappings:

```bash
"$ANDROID_HOME/platform-tools/adb" \
  -s "$DEVICE_SERIAL" reverse --list
```

Remove the local mapping after testing:

```bash
endly -t=removeProxy
```

## 5. Verify the installed app

```bash
endly -t=verify
```

This confirms that Agently is the resumed activity on the chosen Android device. In the app, refresh the workspace and confirm the expected starter tasks load before testing a conversation.

## Verify a long-running conversation

Android submits the turn once and then observes the conversation independently,
like the web client. A successful submission can continue on the server after the
original HTTP response or mobile stream is interrupted.

Expected lifecycle:

1. The submitted user message briefly shows `Sending`.
2. As soon as the server exposes the turn, Android marks the message delivered.
3. One compact progress indicator shows the current narration, or `Assistant is
   thinking…` until narration is available.
4. If the SSE connection is interrupted, the SDK reconnects and hydrates the
   current live state. It does not submit the prompt again.
5. Android shows `failed` only when the server reports a terminal failed turn or
   when it cannot find evidence that the submitted turn was accepted.
6. Progressive `forge-report` and `forge-data` transport remains hidden from the
   transcript. Once committed, the canonical report is rendered as native tabs,
   tables, charts, collections, and callouts.

For a device test, start a report that takes long enough to display narration,
temporarily interrupt and restore network connectivity, then verify that the same
conversation resumes without a duplicate user turn. After completion, reopen the
conversation and compare every report tab with web, including collection rows and
authored warning/danger/info colors.

Useful diagnostics (do not paste authentication material into logs):

```bash
"$ANDROID_HOME/platform-tools/adb" -s "$DEVICE_SERIAL" logcat \
  -s AgentlyAndroid OkHttp
```

The canonical Endly build runs tests for the app and both pinned Android SDKs, so
report-compiler or stream-reconnection regressions fail before APK installation.

## Authentication safety

- Use the app's approved interactive sign-in or an operator-controlled local authentication setup.
- Do not place credentials, session cookies, OOB secret references, or private workspace hostnames in this workflow, Gradle files, documentation, or shell history.
- ADB reverse protects the local route from LAN exposure, but the local Agently server must still enforce authentication and workspace authorization.
- Avoid uninstalling the app or clearing its data during routine deployment because that removes the authenticated session.
