# Publishing changes to GitHub

A step-by-step guide for getting local changes onto
<https://github.com/wittywilla/WittyMediaServer>, written for people new to Git.

**Words you'll see:** a *commit* is a saved snapshot of your changes with a message; *push*
uploads commits to GitHub; a *branch* is a separate line of work; a *release* is a downloadable
version (with the built archives) on GitHub's Releases page.

## Option A — GitHub Desktop (easiest)

1. Install [GitHub Desktop](https://desktop.github.com/) and sign in with your GitHub account.
2. *File → Add local repository…* → choose the `WittyMediaServer` folder.
3. The left panel lists every changed file. Write a summary at the bottom left, e.g.
   `0.2.0-alpha: WittyDVD, TV shows, themes, transcoding settings`, and click
   **Commit to master**.
4. Click **Push origin** at the top. Your changes are now on GitHub.

## Option B — the command line

Run these in a terminal inside the `WittyMediaServer` folder.

One-time setup (use the email of your GitHub account, or your GitHub "noreply" address from
*GitHub → Settings → Emails*):

```bash
git config --global user.name "WittyWilla"
git config --global user.email "you@example.com"
```

Every time you want to publish:

```bash
git status                     # see what changed
git add -A                     # stage everything (the .gitignore keeps build output out)
git commit -m "0.2.0-alpha: WittyDVD, TV shows, themes, transcoding settings"
git push origin master         # upload
```

The first push asks you to sign in: choose *Sign in with your browser*. (Git no longer accepts
your GitHub password on the command line; the browser sign-in or a personal access token from
*GitHub → Settings → Developer settings → Personal access tokens* is used instead.)

## Publishing a release (the downloadable archives)

The repository's release workflow (`.github/workflows/release.yml`) builds every platform when
you push a **version tag**:

```bash
git tag v0.2.0-alpha
git push origin v0.2.0-alpha
```

Then watch the *Actions* tab on GitHub; when the workflow finishes, the archives appear on the
*Releases* page. Mark alpha releases as *pre-release* there. In GitHub Desktop you can create the
tag via *History → right-click the commit → Create tag…*, then *Push origin*.

Before tagging, update the version in `cmd/wms/main.go` and `cmd/wms-tray/main.go` (the default
`version`), and add a section to `CHANGELOG.md`.

## Good habits

- Commit small, related changes together with a message that says *why*.
- Run the checks in [CONTRIBUTING.md](../CONTRIBUTING.md) before pushing; GitHub runs them too
  (*Actions* tab) and shows a red ✗ if something fails.
- Never commit disc images, media, API keys or your data folder. The `.gitignore` already
  excludes build output, databases and logs.
- Bigger changes: create a branch (`git checkout -b my-feature`), push it, and open a *pull
  request* on GitHub so you can review the diff before it lands on `master`.

---
<sub>WittyMediaServer 0.2.0-alpha documentation</sub>
