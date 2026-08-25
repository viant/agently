# Build and deploy the iOS app

The canonical physical-device workflow is [`deployment/mobile/ios/run.yaml`](../deployment/mobile/ios/run.yaml). It uses Xcode automatic signing, CoreDevice `devicectl`, and an explicit workspace endpoint. It never stores authentication credentials or OOB secret references.

## Prerequisites

- A compatible Xcode installation with the phone's iOS device support.
- An Apple Development signing identity and Xcode-managed provisioning profile.
- Developer Mode enabled on the iPhone.
- The iPhone unlocked, trusted, paired, and connected by USB.
- Endly available on `PATH`.

If using an Xcode beta, move it to `/Applications/Xcode-beta.app` or set:

```bash
export AGENTLY_IOS_DEVELOPER_DIR='/absolute/path/to/Xcode-beta.app/Contents/Developer'
```

## Select signing and workspace

Set the team shown in **Signing & Capabilities** and an approved HTTPS endpoint. The endpoint below is intentionally non-operational:

```bash
export AGENTLY_IOS_DEVELOPMENT_TEAM='<APPLE_TEAM_ID>'
export AGENTLY_IOS_BASE_URL='https://workspace.example.com/'
```

When multiple phones are paired, select one by CoreDevice identifier, UDID, serial number, or device name:

```bash
export IOS_DEVICE_ID='<IOS_DEVICE_IDENTIFIER>'
```

## Deploy

```bash
cd <agently-repository>/deployment/mobile/ios
endly -t=deploy
endly -t=verify
```

The workflow runs SDK and app tests, creates a signed device build at:

```text
ios/build/DerivedData/Build/Products/Debug-iphoneos/Agently.app
```

It installs in place, launches Agently with the explicit workspace endpoint, and verifies both installation and the running process.

## Individual tasks

```bash
endly -t=doctor
endly -t=test
endly -t=build
endly -t=install
endly -t=launch
endly -t=verify
```

Installation preserves compatible app data and the persistent session. Do not place cookies, tokens, OOB secret references, or private workspace hostnames in this workflow or documentation.
