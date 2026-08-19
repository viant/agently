# Build and deploy the Android app

Android build and deployment are managed by the single Endly workflow in [`deployment/mobile/android/run.yaml`](../deployment/mobile/android/run.yaml). Do not duplicate its Gradle or ADB steps in another workflow.

Every remote build must receive an explicit workspace endpoint. The examples use the intentionally non-operational placeholder `https://workspace.example.com/`; replace it at execution time with an approved endpoint. Never commit a private workspace hostname, credentials, cookies, or encrypted-secret references.

## Prerequisites

- macOS with JDK 17
- Android SDK and platform tools
- Endly installed and available on `PATH`
- An Android device or emulator visible to `adb`
- For local development, an Agently server and workspace available on the Mac

Run the workflow from its directory:

```bash
cd /Users/awitas/go/src/github.com/viant/agently/deployment/mobile/android
```

## 1. Select a device

List connected targets:

```bash
${ANDROID_HOME:-/Users/awitas/Library/Android/sdk}/platform-tools/adb devices -l
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
cd /Users/awitas/go/src/github.com/viant/agently
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
${ANDROID_HOME:-/Users/awitas/Library/Android/sdk}/platform-tools/adb \
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

## Authentication safety

- Use the app's approved interactive sign-in or an operator-controlled local authentication setup.
- Do not place credentials, session cookies, OOB secret references, or private workspace hostnames in this workflow, Gradle files, documentation, or shell history.
- ADB reverse protects the local route from LAN exposure, but the local Agently server must still enforce authentication and workspace authorization.
- Avoid uninstalling the app or clearing its data during routine deployment because that removes the authenticated session.
