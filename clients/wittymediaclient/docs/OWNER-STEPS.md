# What the owner needs to do

Things only you can do, in the order they make sense. Nothing here has been done for you: no accounts,
no signing and no publishing. Prices are what these programmes have charged and may have changed, so
check the sign-up page before paying.

## Decisions

These change what gets built. Each has a recommendation.

1. **Windows 7.** Supporting it means a second desktop app, frozen on a 2022 browser engine that stopped
   getting security fixes in October 2023. *Recommendation: don't. Windows 7 users can use a browser.*
   See [the design](DESIGN.md#windows-7).
2. **The app's ID.** Android and Apple use `com.wittywilla.wittymediaclient`. Once published it can never
   change, so say now if you want something else.
3. **The name people see.** Currently "WittyMedia".
4. **Artwork.** Every app needs an icon, and TVs need a wide banner too.

## Step 1: Let CI build the apps

CI builds the Android and Apple apps on GitHub's computers, and this is the first time that code will
ever be compiled. **Expect failures the first time**: fixing them is normal.

The native apps need the `client-contract` branch merged too, or they can't connect. Nothing here pushes
or merges, so that's your call. The simplest way is to ask your main Claude session to merge both
branches and push them, once you're happy.

Pushing runs Android's build on GitHub's machines, which accepts the Android SDK licence terms there on
your behalf. Read them at <https://developer.android.com/studio/terms> if you want to check first.

GitHub Actions is free for public repositories. For a private one, the Mac builds use your free minutes
quickly.

## Step 2: Try Android (free, no account)

Once CI is green:

1. On GitHub, open the repository's **Actions** tab, then the latest **wittymediaclient** run.
2. Under **Artifacts**, download **wittymedia-android-sideload**, and unzip it to get an `.apk` file.
3. Put it on the device:
   - **Phone or tablet:** copy it over and open it. Android asks you to allow installing from that app.
   - **Android TV, Google TV, Fire TV:** in Settings, turn on Developer options and network debugging
     (the menu differs by device). Then, from a computer with Android's `adb` tool, run the two commands
     below, using your device's address.

```bash
adb connect 192.168.1.50
```

```bash
adb install app-sideload.apk
```

This build is signed with a throwaway key. It's for testing only, and can't be updated to a store version
without uninstalling first.

**Please check on a real device and report back:** does it start, connect, sign in, and play? Are the
remote and focus usable? Is it smooth? A cheap TV stick is the most useful test there is.

## Step 3: Try Apple (needs a Mac)

Apple only allows building and installing from a Mac. If you don't have one, you can't run these apps
yourself yet, whatever you pay. Borrowing a Mac or renting one online are the usual ways round that.

1. Install **Xcode** from the Mac App Store (free), and open it once.
2. In Terminal, install the tool that creates the Xcode project: `brew install xcodegen`. This needs
   [Homebrew](https://brew.sh).
3. In the `clients/wittymediaclient/apple` folder, run `xcodegen generate`, then open
   `WittyMedia.xcodeproj`.
4. In Xcode's settings, under **Accounts**, add your Apple ID.
5. Choose a target such as **WittyMedia-tvOS**. Under **Signing & Capabilities**, pick your team.
6. Pair your device. For Apple TV: on the TV, go to Settings, Remotes and Devices, Remote App and Devices.
   In Xcode, open Window, Devices and Simulators.
7. Choose the device at the top of the window and press Run.

A free Apple ID should be enough to run your own builds, but they expire after about a week. The first
launch asks permission to find devices on your network, and should be allowed.

## Step 4: Publish, when you're ready

Only worth doing once the apps work on real devices.

| Store | Cost | Reaches |
|---|---|---|
| Apple Developer Program | About $99 a year | App Store and TestFlight, for all Apple devices |
| Google Play Console | About $25, once | Android phones, tablets, Android TV, Google TV |
| Amazon Appstore | Free | Fire TV |
| Microsoft Store | Optional | Windows |
| LG, Samsung | Free developer accounts, as far as is known | webOS and Tizen, when those clients exist |

**Signing keys.** Android needs a signing key made once, kept private and backed up. If it's lost, you
can't update the app, unless you use Google's Play App Signing, which is recommended. Never put a key in
this repository: `.gitignore` already refuses `*.key` files.

**Windows and Mac warnings.** Unsigned desktop apps trigger security warnings when opened. Removing them
needs a code-signing certificate (Windows) or the Apple programme (Mac).

**Store reviews.** Both Apple and Google ask for a way to test a media app. You'd need to give reviewers a
working server, or a demo mode, which doesn't exist yet. Apple also asks why the app uses plain HTTP on the
local network and needs local network access. The reason is that home media servers run that way.

**Consoles.** PlayStation, Xbox and Nintendo all need you to apply to and be accepted by a developer
programme, usually as a company. Nothing can be built until then.
