# Changelog

## [1.2.0](https://github.com/Disble/dharness/compare/v1.1.0...v1.2.0) (2026-08-12)


### Features

* **cli:** run ESLint in the gate, placed by measured cost not assumption ([e253aa6](https://github.com/Disble/dharness/commit/e253aa62f6b72a687cd3cbad726418a377507c36))
* **cli:** run fallow dupes, because audit does not enforce the ceiling ([149b924](https://github.com/Disble/dharness/commit/149b924f1f6e0bcddadbd4d011bb5a46a53840bf))
* **jsconfig:** parse eslint.config.js with a pure-Go tree-sitter parser ([546cf71](https://github.com/Disble/dharness/commit/546cf71048c37b90f6f26f8f228a5df9e95424f9))
* **preset:** layer eslint-config-next and eslint-config-expo into the owned config ([90e22f6](https://github.com/Disble/dharness/commit/90e22f60369f2dd8911b95579aef2d50409a629f))
* **preset:** opine on how fallow looks for duplication, not just how much ([0db3bca](https://github.com/Disble/dharness/commit/0db3bca06e8ba698f92e53c63a8cb7e2dc44a764))
* **setup:** splice or replace eslint.config.js instead of only writing it ([7a40835](https://github.com/Disble/dharness/commit/7a408351a3784f246eb98373ce17db9561787ac8))
* **setup:** write eslint.config.js when a project has none at all ([703f51d](https://github.com/Disble/dharness/commit/703f51d10781c06c04ef7284d9cf3ac2da77af6a))
* **setup:** write the owned ESLint factory config and repair its allow-list entry ([7227829](https://github.com/Disble/dharness/commit/7227829760944b64f786adb4c59fe23f27b7e30a))


### Bug Fixes

* **setup:** name the residue entries the note found, not a disjunction ([d40b995](https://github.com/Disble/dharness/commit/d40b99599ab9a0ebe3f02535e2cd6a771e8da1bb))
* **setup:** report doctor.config.json eslint residue instead of staying silent ([f28ee62](https://github.com/Disble/dharness/commit/f28ee62a83a5b5df4746052b472d85eb8aeea6bd))

## [1.1.0](https://github.com/Disble/dharness/compare/v1.0.2...v1.1.0) (2026-08-11)


### Features

* **preset:** add Next.js and Expo, and let a preset seed the prompt ([72339b3](https://github.com/Disble/dharness/commit/72339b391d88ccd1661f8f3bea279761934b180c))
* **preset:** add the registry and the region dharness rewrites ([53330fa](https://github.com/Disble/dharness/commit/53330fa8b4f783cd9d572507709f4b7c1f66230b))
* **preset:** add the Wails preset and let a match say what it could not read ([542dab1](https://github.com/Disble/dharness/commit/542dab1d9d36841ea863d789cb4dbdb8b7054911))
* **preset:** ship a cross-cutting duplication ceiling through generic ([116f542](https://github.com/Disble/dharness/commit/116f54272f896afa006a13923dc84eb7104678c5))
* **project:** derive folder-ownership's default from the tree, not a constant ([2094e6b](https://github.com/Disble/dharness/commit/2094e6b607cda679ec35d37c2cb46bac41ea5fe8))
* **setup:** report every contributed key the project also declares ([17b9295](https://github.com/Disble/dharness/commit/17b92955bea0475d7e12b560aaccbf0ffe2e9f0c))

## [1.0.2](https://github.com/Disble/dharness/compare/v1.0.1...v1.0.2) (2026-08-11)


### Bug Fixes

* **setup:** report a legacy lint config react-doctor cannot read ([bd9525e](https://github.com/Disble/dharness/commit/bd9525e3b52608b7f2b2e9a2cc378a2795d62e54))
* **setup:** report a project that declares its own fallow boundaries ([308c29e](https://github.com/Disble/dharness/commit/308c29e43e2d54fcabfd627b075d12dfb4e071e8))
* **setup:** ship folder-ownership off and let the project turn it on ([5506ad2](https://github.com/Disble/dharness/commit/5506ad21d13d58ecc4fe9fcded5ec1a7351342e2))

## [1.0.1](https://github.com/Disble/dharness/compare/v1.0.0...v1.0.1) (2026-08-11)


### Bug Fixes

* **setup:** install through the package manager instead of reading node_modules ([3afbe65](https://github.com/Disble/dharness/commit/3afbe65c6cfa60a5aa5d1cbe8b4c84f151cc0a72))

## [1.0.0](https://github.com/Disble/dharness/compare/v0.2.0...v1.0.0) (2026-08-11)


### ⚠ BREAKING CHANGES

* **cli:** dharness init no longer exists. There is one command, sync, which both adopts a repository and brings it up to date — there is no install command and a separate maintenance command (15). Anything invoking dharness init must call dharness sync instead. sync also writes now: the old read-only sync that only reported has been removed.

### Features

* **cli:** merge init into sync and make delegation a per-project answer ([af5f353](https://github.com/Disble/dharness/commit/af5f353c9b9256384d8bb9d4d735268c8dceffd2))

## [0.2.0](https://github.com/Disble/dharness/compare/v0.1.0...v0.2.0) (2026-08-10)


### Features

* run wrapped tools from remote latest ([5ac96c8](https://github.com/Disble/dharness/commit/5ac96c8bb0e2bfbe8b27347ad0843ff610dfd17e))


### Bug Fixes

* **setup:** restore package state after failed init ([01b05c7](https://github.com/Disble/dharness/commit/01b05c78cf56ba61b3e9d5dfbec127fc1b252964))

## 0.1.0 (2026-08-10)


### Features

* give dharness the local gate it installs everywhere else ([c3740c0](https://github.com/Disble/dharness/commit/c3740c0c2217657f490106024305d76ed3413357))
* one plan, two verbs — sync reports it, init applies it ([07926b8](https://github.com/Disble/dharness/commit/07926b8a0c2d45167b059db58c7dd613f1e5ef5e))
* record the measurement so sync can finish ([59d1cdc](https://github.com/Disble/dharness/commit/59d1cdc4b6881f2aaa6f3685dd342d0ba9bfecbb))
* tell the repository apart from the JS project inside it ([450007c](https://github.com/Disble/dharness/commit/450007cbd607b74dab5bd2c49a1a035d6ff0382c))
* wrap react-doctor, fallow and Stryker behind four commands ([420b7fb](https://github.com/Disble/dharness/commit/420b7fbda4442269804f5e4a92b6df9a6e0c17d8))


### Bug Fixes

* budget the dry run, which is where Stryker starts its processes ([2176b7b](https://github.com/Disble/dharness/commit/2176b7b43aada5222f3c930434c2475d1074b24c))
* check out LF on every platform ([e3414a4](https://github.com/Disble/dharness/commit/e3414a429f264bdeb989c042f811204e6adc0aba))
* do not run fallow before there is anything to compare against ([3f2f6c3](https://github.com/Disble/dharness/commit/3f2f6c38caa411b7cb95d9481cd2aef1ef5fa42e))
* give Stryker the paths it will actually read ([c73548f](https://github.com/Disble/dharness/commit/c73548f560f10d51b95b49343c890b16556181ff))
* gofmt check fails on the Windows runner ([a6d04c3](https://github.com/Disble/dharness/commit/a6d04c301d1f36f48667e38e631971a44160b866))
* make mutate mean something, verified against real Stryker ([7120785](https://github.com/Disble/dharness/commit/7120785270083999690ee272bc3fb60f7b8cf71a))
* stop the mutation sandbox leaking into the repository ([15e4c01](https://github.com/Disble/dharness/commit/15e4c0160ad3d7070c7ec7a63fb6a2e3f6547d01))
* sync reports setup, and can say there is nothing left ([51e94ec](https://github.com/Disble/dharness/commit/51e94ec734093d2f52c86515ab1a485e6d89ad5a))

## Changelog

This file is generated by release-please from the conventional commit messages
on main. Nothing here is written by hand: an entry that has to be maintained is
an entry that can disagree with the history it claims to describe.

Commits prefixed `docs:`, `test:` and `chore:` are left out — they change
nothing a person installing this needs to know about.
