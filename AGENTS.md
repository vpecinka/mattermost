# AGENTS.md

Explicitly import subdirectory instruction files that must always be in context:
@server/AGENTS.md

## Cursor Cloud Agents

This repository has a checked-in Cloud Agent environment under `.cursor/`. Docker is started by `.cursor/scripts/cloud-agent-start.sh`; if Docker is unavailable in Cloud, treat that as an environment failure rather than falling back to snapshot assumptions.

The environment declares `mattermost/enterprise` as a Cursor multi-repo dependency. Cursor clones the repositories as siblings, so `server/Makefile` can use its default `../../enterprise` path; the install hook does not clone or symlink enterprise.


Now in czech:

Nacházíš se v repository opensource projektu Mattermost, kde ve větvi szn-build udržuji vlastní rozšíření a implementace jinak licensovaných featur Mattermostu. Moje rozšíření se nacházejí vesměs v adredsáři server/custom, dále pak drobné udržovatelné patch v kódu (např. přeskočení testu na licenci, apod) a to samé ve webapp/ části. V serverové části většinou implementují nejaký interface, který je dostupný, ale jeho implementace ne. Při vývoji respektuj licenční podmínky MAttermostu, které obecně zakazují používat a měnit kód v adresáři enterprise/, jinak zbytek je open source. V hlavičkách mých rozšíření dodržuji podobný styl, tj. zmiň, že jde o copyright Seznam.cz a rozšiřuje to Mattermost. Pokud budeš někde něco komentovat nebo psát .md dokumentaci, vždy používej angličtinu. Nevytvořej příliš mnoho .md dokumentů, jeden pro každé nové rozšíření stačí - v něm uveď, o co jde a jak je dosaženo výsledku. Dokumentaci udržuj aktuální. Pokud asi pustíš terminál, nezapoměň poprvé napsart "nvm list", tím se aktivuje nodejs pro budoucí použití. Při implementaci nejdříve analyzuj stávající kód, pak navrhni jedno nebo dvě ření, snaž se dekomponovat a nepsat podobný kód dvakrát, tkaé se koukni, zda nelze použít už nějaký stávající kód. Než začneš měnit (implementovat) soubory, tak si se mnou potvrď analýzu a navrhnované kroky a směr vývoje.
