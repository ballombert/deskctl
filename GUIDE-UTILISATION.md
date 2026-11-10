# Guide d'utilisation de deskctl

`deskctl` est un outil Windows pour organiser les fichiers du Bureau et placer les fenêtres sur des bureaux virtuels. Il propose un atelier graphique Windows, une interface interactive dans le terminal (TUI) et des commandes en ligne de commande (CLI).

## 1. Préparer l'outil

Ouvrir PowerShell à la racine du dépôt, puis se placer dans le dossier du programme :

```powershell
Set-Location tools\deskctl
```

Pour lancer le programme depuis le code source, Go 1.26.2 ou une version compatible est nécessaire :

```powershell
go run . tui
```

Pour créer un exécutable :

```powershell
go build -o deskctl.exe .
.\deskctl.exe --help
```

Les chemins des fichiers JSON utilisés ci-dessous sont relatifs au dossier courant. Les exemples partent de `tools\deskctl`.

### Utiliser l'atelier graphique

Avec Go, Node.js 24 et le runtime WebView2 installés :

```powershell
.\build-gui.ps1
.\deskctl-gui.exe
```

L'écran **Studio** regroupe tout : fenêtres à gauche, bureaux et écrans au centre,
groupes et règles à droite.

1. Rechercher une fenêtre par son titre ou son application.
2. Glisser la fenêtre vers la colonne d'un bureau (ou **Tous les bureaux** pour
   l'épingler). Au clavier, sélectionner la fenêtre puis choisir une destination
   dans l'inspecteur. Aucun déplacement n'a encore lieu.
3. Pour réutiliser ce choix, cliquer sur **En faire une règle**, adapter les
   expressions et vérifier le compteur de correspondances avant d'enregistrer.
4. Pour un groupe tuilé : Ctrl+clic sur plusieurs fenêtres, puis **Grouper côte à
   côte ▥** ou **Grouper empilées ▤**. L'éditeur s'ouvre avec la disposition
   actuelle ; ajuster les tailles en glissant les séparateurs ou dans **Taille %**.
   **Enregistrer ce groupe** conserve ses critères, même si les fenêtres sont
   ensuite fermées. Créer une règle et sélectionner ce **Groupe réutilisable**
   pour définir sa destination. Plusieurs règles peuvent référencer le même groupe.
5. Survoler une règle pour voir sa disposition en pointillés sur le canvas.
   Cliquer **Appliquer** sur la règle, ou **Tout appliquer** dans la barre du haut,
   puis confirmer. Une identité ou un bureau changé depuis l'aperçu entraîne
   l'abandon de cette affectation, avec explication.
6. Pour revenir en arrière : **Annuler le dernier lot** (bureaux) ou **Restaurer
   les positions** (dispositions).

### Écrans et FancyWM

La barre des écrans, au-dessus du canvas, liste les moniteurs. Le sélecteur
**FancyWM** associe chaque écran à son indice `DisplayN` et l'enregistre dans
`deskctl.yaml`. **Envoyer sur …** dans l'inspecteur déplace la fenêtre : avec
FancyWM si l'écran a un indice et que FancyWM est lancé, sinon en conservant sa
position relative.

Pour une fenêtre appartenant à un groupe non ambigu, **Envoyer sur …** prépare
la destination du groupe entier au lieu de déplacer immédiatement cette fenêtre.

Le fichier peut aussi être modifié à la main :

```yaml
version: 1
fancywm:
   executable: fancywm
   display_indices:
      '\\.\DISPLAY1': 1
tiling:
   engine: auto   # auto | fancywm | native
   gap: 8         # marge en pixels entre fenêtres (0-64)
```

Les clés doivent correspondre aux IDs montrés par deskctl. Le bouton
**Paramètres d'affichage Windows** ouvre les réglages natifs pour l'écran principal,
la disposition, la résolution et l'orientation. Ces modes ne sont pas modifiés
directement par deskctl.

La destination **Tous les bureaux** épingle une fenêtre sur chaque bureau virtuel.
Une règle de groupe épinglée épingle toutes les fenêtres du groupe. L'état épinglé
est inclus dans la vérification et le journal ; **Annuler le dernier lot** peut
donc désépingler la fenêtre et la replacer sur son bureau précédent.

La GUI classe chaque fenêtre séparément. La TUI et les layouts CLI historiques
continuent de regrouper les fenêtres par PID.

### Écrire une règle

Les conditions sont des expressions régulières insensibles à la casse.
`application` est le nom du processus sans `.exe` : `^Code$`, `^(msedge|chrome)$`.
`title` cherche dans le titre : `deskctl`, `^Réunion`. Un champ vide ne filtre pas ;
chaque fenêtre de règle exige au moins une condition. Les règles contradictoires ne
décident pas silencieusement à votre place : la fenêtre est signalée en conflit.
Le choix manuel d'un bureau résout le conflit pour ce lot uniquement.

Dans **Groupes**, **Nouveau groupe** crée un arbre même sans fenêtre ouverte :
renseigner les critères application/titre des feuilles. H = côte à côte ; V =
empilé. Les sous-groupes et proportions restent disponibles (4 niveaux).
L'ajout de la sélection utilise aussi le titre pour distinguer plusieurs fenêtres
d'une même application. Vérifier les correspondances avant d'enregistrer.

Dans une règle, choisir un **Groupe réutilisable**, un bureau et un écran.
Son arbre est partagé et non modifiable localement : utiliser **Modifier le
groupe partagé**. **Disposer en tuiles** reste disponible pour créer une disposition
qui sera convertie en groupe partagé à l'enregistrement. Le fichier v3 contient
ensemble `rules` et `groups` ; import/export transportent les deux catalogues.
Supprimer un groupe encore référencé est refusé avec les noms des règles.

### Zone et déplacement d'un groupe

Dans **Préparer la destination de …**, choisir bureau, écran et zone, puis
**Préparer le groupe …**. Les coordonnées sont des pourcentages de la zone de
travail (hors barre des tâches). La zone doit rester dans l'écran. Une règle
peut remplacer la zone par défaut du groupe ; sans zone, le groupe occupe toute
la zone de travail.

On peut aussi glisser la poignée nommée du cadre vers un écran précis dans la
colonne voulue, ou glisser son coin inférieur droit pour préparer sa taille.
Glisser un membre non ambigu prépare le groupe entier. Un conflit d'appartenance
interdit tout déplacement individuel silencieux. Ces gestes ne déplacent rien :
**Tout appliquer**, puis confirmation, reste nécessaire. Ils ne réécrivent pas
les règles. **Tous les bureaux** épingle les membres, sans les dupliquer.

### Suivi Windows, optionnel

Cocher **Suivre les déplacements Windows (session)** pour autoriser les translations
du groupe lorsque l'on déplace une de ses vraies fenêtres. Désactivé par défaut,
le suivi n'est ni enregistré dans les règles ni activé par import. Il ne suit
pas les redimensionnements ou changements de bureau effectués hors de deskctl.

Une notification de suivi marque le plan **périmé** et bloque son application.
Les choix manuels et brouillons restent affichés. Utiliser **Actualiser** ;
une confirmation protège les choix non enregistrés. Les erreurs de suivi sont
affichées explicitement. Dans un navigateur sans runtime Wails, la case est
indisponible : cela ne constitue pas un suivi Windows actif.

Si FancyWM est lancé, le suivi est également désactivé avec explication :
arrêter FancyWM pour l'activer. Cette précaution ne bloque pas la préparation
manuelle des groupes ni leurs dispositions natives en zone partielle.

### Fenêtre qui ne bouge pas

- Badge **Admin** : l'application tourne en administrateur ; relancer
  `deskctl-gui.exe` en administrateur pour la redimensionner.
- Statut **Bloquée** : l'inspecteur indique la raison (fenêtre système ou modale,
  processus illisible, hors bureau virtuel).
- Le bilan précise le moteur utilisé (`fancywm` ou `native`) pour chaque disposition.

Les règles et le dernier journal de fenêtres sont enregistrés près de
`deskctl-gui.exe` dans `window-rules.json`, `window-undo.json` et
`window-geometry-undo.json` (positions avant disposition). Utiliser un dossier
accessible en écriture. Si ce dossier est synchronisé dans le cloud, les JSON
locaux peuvent être synchronisés par votre outil de fichiers ; deskctl ne les
transmet pas à un service distant. Le journal contient les titres et identités
nécessaires à la restauration : le conserver localement et ne pas le partager.

Le journal précédent est remplacé au prochain lot réel, après confirmation.
L'annulation ne restaure que les mouvements confirmés dont la fenêtre n'a pas
changé et dont le bureau d'origine existe encore. En cas d'état incertain ou
d'erreur de journalisation, conserver les données de récupération affichées et
vérifier les bureaux avant de relancer.
Un état incertain enregistré bloque le lot suivant pour préserver la récupération.
Après vérification manuelle, archiver le journal sous un autre nom avant de
recommencer ; un journal illisible doit également être conservé, pas écrasé.

Les mouvements de fenêtres utilisent des interfaces Windows dont certaines
sont internes et non documentées. Un refus de droits ou une version incompatible
reste une erreur visible ; aucun résultat fictif n'est annoncé.

## 2. Utiliser l'interface interactive

Lancer :

```powershell
.\deskctl.exe tui
```

Dans le menu, choisir l'une des opérations :

- **Fichiers du Bureau** : préparer le classement des fichiers et raccourcis.
- **Inventaire des fenêtres** : afficher les fenêtres détectées et, sur demande, exporter l'inventaire.
- **Placement des fenêtres** : déplacer des fenêtres selon un fichier de disposition existant.
- **Annuler un rangement de fichiers** : restaurer des fichiers à partir d'un journal d'annulation.

### Touches principales

- `Haut` / `Bas` : naviguer dans le menu ou faire défiler un aperçu.
- `Entrée` : ouvrir une opération ou passer à l'étape suivante.
- `Tab` / `Maj+Tab` : passer d'un champ de chemin à l'autre.
- `s` : lancer une simulation depuis un aperçu.
- `e` : exporter les JSON proposés depuis un aperçu.
- `a` : demander l'application réelle après la simulation.
- `o` : confirmer l'application réelle.
- `n`, `Entrée` ou `Échap` : refuser la confirmation.
- `Échap` : revenir au menu ; `q` ou `Ctrl+C` : quitter.

L'interface affiche les destinations et les conflits avant l'action. Les exports TUI ne remplacent jamais un fichier existant : choisir un autre chemin si nécessaire. Un terminal d'au moins 32 colonnes sur 16 lignes est requis.

## 3. Ranger les fichiers du Bureau

Le rangement déplace les fichiers et raccourcis dans des dossiers de catégories. Il ne réorganise pas les icônes à l'écran. Les dossiers déjà présents sur le Bureau ne sont pas déplacés.

### Parcours CLI recommandé

1. Scanner le Bureau :

```powershell
.\deskctl.exe desktop scan --out desktop-files.json
```

1. Générer un plan avec les règles fournies en exemple :

```powershell
.\deskctl.exe desktop plan --input desktop-files.json --rules examples\rules.json --out desktop-plan.json
```

1. Examiner le plan sans déplacer de fichier :

```powershell
.\deskctl.exe desktop apply --plan desktop-plan.json
```

1. Si le résultat convient, appliquer avec un nouveau journal :

```powershell
.\deskctl.exe desktop apply --plan desktop-plan.json --execute --undo-log undo-2026-11-09.json
```

1. Vérifier d'abord l'annulation en simulation :

```powershell
.\deskctl.exe desktop undo --log undo-2026-11-09.json
```

1. Pour restaurer réellement les fichiers :

```powershell
.\deskctl.exe desktop undo --log undo-2026-11-09.json --execute
```

`desktop apply` et `desktop undo` sont en simulation par défaut. L'option `--execute` est nécessaire pour déplacer ou restaurer réellement les fichiers.

### Classement

Les règles par défaut classent notamment les documents, images, archives, installateurs et raccourcis selon leur extension. Les extensions inconnues vont dans `Autres`. Le fichier [examples/rules.json](examples/rules.json) peut être adapté.

Si le Bureau est dans OneDrive, les déplacements seront synchronisés. Les fichiers cachés, système, `desktop.ini`, les liens et les éléments déjà dans des dossiers de catégorie ne sont pas traités. En cas de collision, le fichier n'est pas écrasé. Les fichiers modifiés ou disparus depuis le scan sont ignorés par prudence.

Le journal d'annulation doit être un nouveau fichier qui n'existe pas déjà. Le programme refuse de commencer l'application réelle si le chemin du journal n'est pas disponible.

## 4. Placer les fenêtres sur des bureaux virtuels

Le déplacement utilise l'API COM publique `IVirtualDesktopManager`; la liste et les noms utilisent `IVirtualDesktopManagerInternal`, une interface interne non documentée et dépendante de la version de Windows. Aucun programme externe n'est requis.

Pour créer rapidement le layout, lancer `.\deskctl.exe tui`, choisir **Créer une disposition de fenêtres**, puis renseigner les deux chemins de sortie. Le panneau gauche liste les applications et le nombre de fenêtres; le panneau droit affiche les noms de tous les bureaux, y compris les bureaux vides. Utiliser `Haut` / `Bas` pour choisir une application, `Gauche` / `Droite` pour choisir le bureau, puis `Entrée` pour affecter la cible. Toutes les fenêtres du PID reçoivent cette cible. `e` enregistre le layout et l'inventaire de référence après l'aperçu. Les fichiers existants ne sont jamais écrasés.

La création du layout ne déplace aucune fenêtre. L'application réelle reste une étape séparée. Les noms utilisent des interfaces COM internes; une mise à jour Windows peut en changer le fonctionnement.

1. Exporter l'inventaire courant des fenêtres :

```powershell
.\deskctl.exe windows export --out windows.json
```

1. Créer `layout.json` avec les PID de cet inventaire et le `TargetDesktopId` du bureau choisi. Le nom est affiché par la TUI; le fichier conserve son GUID. Exemple :

```json
[
  {
    "ProcessId": 1234,
    "TargetDesktopId": "b8c5a6e2-7f10-4d3a-9c2b-1234567890ab"
  }
]
```

Chaque PID doit être présent une seule fois dans le fichier de disposition. Le PID de l'exemple est fictif : utiliser ceux de votre propre fichier `windows.json`. Un exemple est disponible dans [examples/layout.json](examples/layout.json).

L'inventaire contient une entrée par fenêtre avec `WindowId`, `ProcessId`, `DesktopName` et `DesktopId`. Plusieurs fenêtres peuvent partager un PID; le layout ne contient qu'une cible par PID. Réexporter tout ancien inventaire sans `WindowId` et recréer les layouts utilisant `TargetDesktopName`.

1. Simuler le placement :

```powershell
.\deskctl.exe windows apply --layout layout.json --windows windows.json
```

1. Si le bilan est correct, appliquer réellement :

```powershell
.\deskctl.exe windows apply --layout layout.json --windows windows.json --execute
```

Conserver `windows.json` comme inventaire de référence : le programme compare le nom du processus associé à chaque PID pour éviter de déplacer un autre processus si Windows a réutilisé ce PID. Les processus déjà fermés sont ignorés. Un GUID périmé peut être refusé par l'API; son HRESULT apparaîtra alors dans le bilan.

## 5. Sorties et précautions

- Sans `--execute`, les commandes `desktop apply`, `desktop undo` et `windows apply` simulent l'action.
- Les commandes CLI écrivent leurs JSON aux chemins indiqués et peuvent remplacer les fichiers existants. Utiliser de nouveaux noms de fichiers pour conserver les exports précédents.
- Le rangement n'efface aucun fichier et ne remplace pas une destination déjà présente.
- Quitter le programme ou fermer le terminal pendant une opération n'annule pas les déplacements déjà réalisés.
- Après une erreur pendant l'écriture ou la vérification du journal d'annulation, ne pas relancer le rangement avant d'avoir examiné les informations de récupération affichées.

Pour la description détaillée des protections, des formats et des tests, voir le [README](README.md).
