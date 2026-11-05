# WittyDisc: update plan

> **Status on 2026-11-04**
>
> **In progress:** the server reading disc images with WittyDisc's own reader ([#104](https://github.com/wittywilla/WittyMediaServer/issues/104), [#107](https://github.com/wittywilla/WittyMediaServer/issues/107), from [#98](https://github.com/wittywilla/WittyMediaServer/issues/98)). **Planned:** the rest; no idea posted.

**Now:** 0.2.0. Library, API 3. Reads DVD and Blu-ray images without mounting them (ISO 9660, UDF up to 2.50 with metadata partitions and mirror copies), safe with damaged images. Reaches servers inside WittyDVD and WittyBluray. Pure Go; tests run on Linux; never run on a real Mac or on FreeBSD.

**Channel:** alpha (it rides with the modules built on it).

| Release | Theme |
|---|---|
| 0.2.x | Fixes found through WittyDVD and WittyBluray |
| **0.3.0** | Speed on network shares; more fuzzing; icon |
| Later | More image formats |

## 0.3.0

- **Reading over network shares:** measure seeks and reads on SMB and NFS, add read-ahead and a small block cache tuned for menus (many small reads) and for playback (long runs).
- **Fuzzing** every parser continuously (Go's native fuzzing in CI nightly), with a corpus from damaged and odd images.
- **Icon:** the same disc with no letter, if the owner wants the set.

## Later

- Compressed images people use for archiving (CHD for optical discs is used by game emulators; NRG, MDF/MDS), read-only, where formats are documented and licences fit.
- Run the tests on a real Mac and FreeBSD.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
