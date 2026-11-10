# deskctl : Automatisation & Organisation de l'Espace de Travail Windows 11

`deskctl` est un outil modulaire en Go avec commandes CLI, interface terminal Bubble Tea et application graphique Windows Wails + React, conçu pour :

1. **Inventorier** les fenêtres et les affecter à des bureaux nommés via `IVirtualDesktopManager` et les interfaces COM internes de Windows.
2. **Ranger les fichiers du Bureau** Windows dans des dossiers thématiques de manière totalement sécurisée, avec support natif des Bureaux redirigés (OneDrive) et commande d'annulation (`undo`).

Pour les procédures pas à pas, voir le [Guide d'utilisation](GUIDE-UTILISATION.md).
Le [plan de conception des groupes de fenêtres](docs/plans/groupes-fenetres.md) documente les choix et les étapes de cette fonctionnalité.

## Atelier graphique des bureaux

Avec Go 1.26.2, Node.js 24 et le runtime Microsoft Edge WebView2 :

```powershell
.\build-gui.ps1
.\deskctl-gui.exe
```

Le script installe les dépendances frontend verrouillées, vérifie TypeScript,
compile React et embarque ses fichiers dans `deskctl-gui.exe`. Aucun helper .NET
ni code de FancyWM n'est intégré ; la GUI appelle seulement son CLI (actions en liste
blanche) pour déplacer et disposer les fenêtres. La compilation historique `go build -o deskctl.exe .`
reste indépendante du frontend.

La configuration GUI portable est `deskctl.yaml`, à côté de l'exécutable. Un fichier
absent utilise les valeurs par défaut ; un fichier YAML invalide bloque le démarrage.
`fancywm.executable` accepte un nom présent dans `PATH` ou un chemin complet.
`fancywm.display_indices` associe l'ID Windows exact de chaque moniteur à l'indice
`DisplayN` de FancyWM. Ne pas renseigner d'association incertaine : sans indice,
deskctl place la fenêtre lui-même (moteur natif). La section `tiling` choisit le moteur
de disposition et la marge (voir plus bas).

### Écran unique « Studio »

Tout se fait sur un seul écran, en trois zones :

- **Fenêtres** (gauche) : recherche, filtres (à déplacer, libres, bloquées), badge
  **Admin** pour les applications élevées. Clic = sélection ; Ctrl+clic = sélection
  multiple. L'inspecteur permet de choisir le bureau, d'**envoyer sur un écran** et
  **En faire une règle**. Avec plusieurs fenêtres : **Grouper côte à côte ▥** ou
  **Grouper empilées ▤** crée un groupe réutilisable à partir de leur disposition actuelle
  (tailles en % calculées).
- **Canvas** (centre) : une colonne par bureau, plus **Tous les bureaux**. Chaque colonne
  contient la maquette des moniteurs à l'échelle réelle et les fenêtres en tuiles.
  Glisser une fenêtre sur une colonne prépare le déplacement. Au survol d'une règle,
  la disposition proposée s'affiche en pointillés. La barre des écrans règle l'indice
  FancyWM de chaque moniteur (enregistré dans `deskctl.yaml`) et ouvre les
  **Paramètres d'affichage Windows**.
- **Règles** (droite) : nombre de fenêtres correspondantes, déplacements et disposition
  en attente, boutons **Aperçu**, **Appliquer** (cette règle seule), modifier, supprimer,
  activer/désactiver.
- **Groupes** (droite) : créer un groupe même sans fenêtre ouverte, modifier ses
  critères, son nom, son arbre partagé et sa zone par défaut. Une suppression
  référencée est refusée en indiquant les règles concernées.

**Les affectations de bureau et gestes de groupe préparent seulement le plan.**
L'envoi sur écran d'une fenêtre hors groupe reste immédiat.
**Tout appliquer** (ou **Appliquer** sur une
règle), suivi d'une confirmation, déclenche les déplacements de bureau puis les
dispositions. **Restaurer les positions** remet les fenêtres à leur position d'avant la
dernière disposition (`window-geometry-undo.json`).

### Règles v3 : groupes partagés, bureau, écran et zone

Une règle cible un bureau (ou **Tous les bureaux**, qui épingle toutes ses fenêtres),
éventuellement un écran, et :

- soit une condition simple : `application` et/ou `title` ;
- soit une référence `groupId` à un groupe nommé du catalogue `groups`. Son arbre
  `layout` contient des fenêtres et des sous-groupes `h` (côte à côte) ou `v`
  (empilés, 4 niveaux maximum), chacun avec une taille `weight` en %.

Plusieurs règles partagent le même groupe. Modifier ce groupe change leur arbre
commun ; l'éditeur de règle affiche cet arbre sans permettre de le modifier localement.
La règle choisit écran et bureau, avec une `zone` de remplacement facultative.
Les zones `{x,y,width,height}` sont des fractions de la **zone de travail** de
l'écran (0 à 1), pas des pixels. Sans zone, toute la zone de travail est utilisée.
Le formulaire les affiche en pourcentages.

Les conditions sont des **expressions régulières** (RE2, insensibles à la casse) ; un
champ vide ne filtre pas. `application` est le nom du processus sans `.exe`
(`^Code$`, `^(msedge|chrome)$`). L'éditeur affiche l'arbre en miniature : cliquer une
zone pour la régler, glisser les séparateurs (ou flèches du clavier) pour changer les %,
diviser une zone, ajouter des sous-groupes ou la sélection courante. Le nombre de
fenêtres correspondantes et l'aperçu fantôme se mettent à jour en direct.
**Enregistrer et appliquer** enchaîne les deux actions.

Une fenêtre visée par des groupes concurrents est signalée en conflit et n'est pas
déplacée isolément. Des règles actives donnant des destinations différentes au même
groupe produisent aussi un conflit. Les branches sans fenêtre sont repliées ; les
critères persistent pour retrouver les fenêtres rouvertes.
Les anciens documents sont migrés vers v3 avec sauvegarde ; conserver cette copie
pour revenir à un ancien exécutable, qui ne sait pas lire v3.
`examples\window-rules.json` contient deux exemples désactivés, dont un groupe imbriqué
60 % | (65 % / 35 %) ; remplacer le GUID par un bureau existant.

### Déplacer et suivre un groupe

Glisser sa poignée nommée vers un **écran précis** dans une colonne de bureau,
y compris **Tous les bureaux**. Glisser un membre non ambigu déplace son groupe
entier dans le plan. Le coin inférieur droit prépare un redimensionnement.
L'alternative clavier est **Préparer la destination de …** : bureau, écran et
zone numériques, puis **Préparer le groupe …**. Une seule révision est préparée ;
aucune fenêtre ne bouge et aucune règle n'est réécrite avant application explicite.

**Suivre les déplacements Windows (session)** est désactivé par défaut. Son activation
explicite autorise le suivi des translations de fenêtres réelles pendant cette session,
pas l'application automatique des règles, ni le suivi des redimensionnements et des
changements de bureau externes. Le suivi nécessite la liaison d'événements Wails ;
son absence et les erreurs sont affichées. Après une notification, le plan est
marqué périmé sans effacer les choix ou brouillons : **Actualiser** est obligatoire
avant application. L'import remplace ensemble groupes et règles, sans activer le suivi.

Le suivi est indisponible tant que FancyWM est actif : deskctl ne peut pas vérifier
de façon fiable quelles fenêtres lui appartiennent en flottant. Arrêter FancyWM
avant d'activer le suivi. Les déplacements et dispositions manuels de groupes
restent disponibles, notamment via le placement natif pour les zones partielles.

### Moteurs de disposition

`tiling.engine` dans `deskctl.yaml` (ou le sélecteur **Moteur** du Studio) :

- `auto` (défaut) : FancyWM s'il est lancé, sinon placement natif ;
- `fancywm` : séquence `MoveToDisplayN`, `CreateHorizontalPanel`/`CreateVerticalPanel`,
  `MoveLeft/Up`, puis ajustement des tailles par pas de 1/12 et vérification ;
- `native` : `SetWindowPos` exact, compensé pour les bordures invisibles et le DPI.
  Si FancyWM gère la fenêtre, deskctl la passe d'abord en flottante.

FancyWM n'agit que sur la fenêtre au premier plan du bureau courant : il n'est utilisé
que si la disposition vise le bureau courant (ou est épinglée), que l'écran a un indice
FancyWM et qu'aucune fenêtre n'est élevée. Sinon, ou en cas d'échec, le moteur natif
prend le relais (indiqué dans le bilan). `tiling.gap` fixe la marge en pixels (0–64).

### Fenêtres « non déplaçables »

Causes corrigées ou expliquées :

- l'API publique de Windows refuse de déplacer les fenêtres d'autres processus ;
  deskctl utilise désormais l'interface interne `MoveViewToDesktop` ;
- les titres dynamiques (navigateur, Teams…) ne bloquent plus : l'identité repose sur
  HWND, PID, heure de démarrage, classe et application ;
- les fenêtres d'applications **administrateur** ne peuvent pas être redimensionnées
  par un processus standard : lancer `deskctl-gui.exe` en administrateur ;
- les fenêtres système ou modales refusées par Windows sont marquées **Bloquée** avec
  la raison.

### Journaux et sécurité

Le dernier lot utilise `window-undo.json`, distinct de l'annulation des fichiers.
**Annuler le dernier lot** prépare une restauration, puis demande confirmation.
Les fenêtres fermées, modifiées ou déplacées manuellement sont ignorées ; un bureau
supprimé n'est jamais recréé. Un lot réel remplace le journal précédent après
confirmation. Les déplacements incertains après une erreur ou une interruption
exigent une vérification manuelle et ne sont pas annulés aveuglément.

L'application vérifie l'identité et le bureau avant chaque mouvement,
puis relit la destination. Elle ignore ses propres fenêtres et celles dont l'identité
ou le bureau est indisponible. Les fenêtres épinglées sont reconnues séparément et
peuvent être épinglées ou désépinglées depuis l'aperçu ; leur état est journalisé pour
permettre l'annulation. Les erreurs COM, de droits ou de journalisation
restent visibles. Si le dossier portable n'est pas accessible en écriture, aucun
repli silencieux n'est effectué. Une erreur de journalisation interrompt les
mouvements suivants et affiche les données de récupération.
Un journal contenant des mouvements incertains bloque tout nouveau lot : vérifier
les bureaux, conserver une copie du journal puis l'archiver hors de
`window-undo.json` avant de recommencer. Un journal illisible n'est pas écrasé.

Tout reste local. Aucun suivi permanent ni déplacement au démarrage. L'actualisation
prévient avant d'abandonner les choix non enregistrés. La compatibilité des interfaces
COM internes reste dépendante de Windows ; un inventaire réussi ne garantit pas
que tous les déplacements interprocessus sont autorisés.

## Interface Terminal Interactive (Bubble Tea)

Depuis `tools/deskctl` :

```powershell
go run . tui
# Ou après compilation :
.\deskctl.exe tui
```

Les commandes CLI restent disponibles. `windows apply` n'utilise plus `--bin`; le layout stocke `TargetDesktopId`, tandis que la TUI affiche le nom du bureau. Sans argument, l'outil affiche toujours son aide.

Le menu propose cinq entrées :

- **Fichiers du Bureau** : scan, aperçu des destinations et conflits, simulation, puis rangement après confirmation.
- **Inventaire des fenêtres** : aperçu des processus et titres, puis export JSON sur demande.
- **Créer une disposition de fenêtres** : voir les applications et les bureaux nommés dans deux panneaux, choisir une cible, puis exporter le layout et l'inventaire.
- **Placement des fenêtres** : lecture d'un layout JSON existant, simulation, puis application après confirmation.
- **Annuler un rangement de fichiers** : aperçu d'un journal existant, simulation de restauration, puis confirmation. Cette action ne restaure pas les fenêtres.

### Navigation

- `Haut` / `Bas` : sélectionner une entrée ; `Entrée` : ouvrir.
- Dans les paramètres, `Tab` / `Maj+Tab` : parcourir les chemins ; `Entrée` prépare l'aperçu.
- Dans la création de disposition, le panneau gauche liste les applications et le panneau droit les bureaux par nom. `Haut` / `Bas` choisissent une application; `Gauche` / `Droite` choisissent un bureau; `Entrée` affecte la cible.
- Dans un aperçu, `s` lance une simulation. `e` exporte les fichiers lorsque cette action est proposée.
- Dans l'aperçu d'une disposition, `e` enregistre le layout et son inventaire ; `m` permet de modifier les affectations.
- Après une simulation, `a` ouvre la confirmation. Seul `o` lance l'action réelle ; `n`, `Entrée` ou `Échap` refusent.
- `Échap` : retour au menu ; `q` ou `Ctrl+C` : quitter.

Les chemins relatifs sont résolus depuis le dossier de lancement. Les paramètres utilisent les noms habituels : `rules.json`, `desktop-files.json`, `desktop-plan.json`, `windows.json`, `layout.json` et `undo-log.json`. Un fichier de règles absent utilise le classement par défaut ; un fichier de règles invalide affiche une erreur.

**Les exports et les journaux existants ne sont jamais écrasés par la TUI.** Choisir un nouveau chemin si le fichier existe. L'export est facultatif : scanner, préparer et simuler ne créent aucun JSON. Si l'export du scan réussit mais celui du plan échoue, le bilan indique le fichier déjà enregistré.

Le rangement exige un nouveau journal accessible avant tout déplacement. Après l'exécution, le journal est écrit puis relu pour vérifier son contenu. Une erreur de journalisation affiche les résultats partiels et les données de récupération ; ne pas relancer le rangement dans ce cas.

Le déplacement utilise l'API COM publique `IVirtualDesktopManager`. L'énumération et les noms utilisent `IVirtualDesktopManagerInternal`, une interface COM interne non documentée et dépendante de la version de Windows. La liste inclut aussi les bureaux vides. Le layout stocke les GUID; l'interface et les bilans affichent les noms. Les inventaires sans `DesktopId` et les layouts avec `TargetDesktopName` doivent être recréés. Un GUID peut changer si un bureau est supprimé puis recréé.

L'écran signale le mode courant, l'activité, les conflits, les éléments ignorés et les échecs. Les détails des moteurs sont disponibles **à la fin** du traitement : aucun pourcentage fictif ni suivi par fichier en temps réel. Pendant un traitement, les actions clavier sont suspendues pour éviter un double lancement. Fermer le terminal ou interrompre le processus ne constitue pas une annulation des déplacements déjà effectués.

Le Bureau résolu et l'avertissement OneDrive sont affichés avant toute action concernée. L'interface se redimensionne et demande au minimum **32 colonnes et 16 lignes**. Aucun déplacement ne peut être confirmé lorsque le terminal est trop petit.

## 🏗️ Architecture & Philosophie Modulaire

```text
tools/deskctl/
├── go.mod
├── main.go                            # CLI principal (windows, desktop)
├── internal/
│   ├── winapi/                        # Win32 pur (EnumWindows, Cloaked DWM, KnownFolders)
│   │   ├── windows.go                 # Vues applicatives, HWND et GUID de bureau
│   │   ├── virtual_desktop.go         # COM public/interne, GUID et noms des bureaux
│   │   └── knownfolders.go            # Résolution Bureau (support OneDrive et Registre)
│   ├── vdesktop/                      # Validation et moteur de layout GUID
│   │   ├── vdesktop.go                # Simulation et application par HWND
│   │   └── vdesktop_test.go           # Tests de validation et simulation
│   ├── tui/                           # Modèle Bubble Tea, opérations et tests
│   └── desktopfiles/                  # Moteur de tri et sécurité de fichiers
│       ├── types.go                   # Structures JSON (Scan, Plan, Rules, Undo)
│       ├── scan.go                    # Scan filtré (ignore desktop.ini, cachés, symlinks)
│       ├── rules.go                   # Moteur de règles (catégorisation par extension)
│       ├── plan.go                    # Calcul du plan et détection des collisions
│       ├── apply.go                   # Déplacement sécurisé et journalisation
│       ├── undo.go                    # Restauration inverse sécurisée
│       └── desktop_test.go            # Tests unitaires complets (collisions, undo)
├── examples/                          # Exemples de fichiers JSON
│   ├── windows.json
│   ├── layout.json
│   ├── rules.json
│   ├── desktop-files.json
│   ├── desktop-plan.json
│   └── undo-log.json
├── prompts/
│   └── categorize-prompt.md           # Prompt LLM durci contre l'injection de titres
└── README.md
```

## 🪟 1. Inventaire des Fenêtres (`windows export`)

### Commande

```cmd
deskctl windows export --out windows.json
```

### Comportement & Filtrage

- Énumère les vues applicatives via `IApplicationViewCollection.GetViews`, y compris sur les bureaux virtuels inactifs.
- Recense `ProcessId`, `WindowId`, `ProcessName`, `WindowTitle`, `DesktopId` et `DesktopName`; les fenêtres sans titre restent dans l'inventaire.
- Plusieurs fenêtres peuvent avoir le même PID. Le layout regroupe ces fenêtres par PID et applique une seule cible à tout le groupe.
- Les fenêtres système sans vue applicative et les PID 0/4 ne sont pas inclus.

### Limite documentée

Un processus peut posséder plusieurs fenêtres. L'inventaire les liste toutes, même si plusieurs fenêtres partagent le même PID. Le layout garde une seule cible par PID; l'affectation déplace toutes les fenêtres de ce processus ensemble. Les fenêtres système sans vue applicative ne sont pas incluses.

## 🧠 2. Catégorisation Externe (Prompt LLM)

Le fichier `prompts/categorize-prompt.md` utilise `DesktopName` pour lire les bureaux et `DesktopId` pour produire les cibles.

### Propriétés du prompt

- Regroupe les fenêtres par projet ou activité à partir du processus et du titre.
- Associe un `DesktopName` à son `DesktopId`; n'invente jamais d'identifiant.
- Chaque PID distinct apparaît exactement une fois dans le layout, même si plusieurs fenêtres du PID sont présentes dans l'inventaire.
- Traite les titres de fenêtres comme des données non fiables, jamais comme des instructions.
- Produit uniquement du JSON brut.

## 🚀 3. Placement sur Bureaux Virtuels (`windows apply`)

### Commandes

```cmd
# 1. Simulation (par défaut)
deskctl windows apply --layout layout.json

# 2. Exécution réelle
deskctl windows apply --layout layout.json --execute
```

Options complémentaires :

### Sécurité & API Windows

- `IVirtualDesktopManager` lit l'identifiant d'une fenêtre et la déplace vers un GUID connu.
- `IVirtualDesktopManagerInternal` énumère les bureaux et lit leurs noms; cette interface n'est pas documentée par Microsoft et peut changer selon le build Windows.
- La sélection inclut les bureaux vides; le layout persiste `TargetDesktopId`. Les anciens inventaires et layouts doivent être recréés.
- Les processus fermés sont ignorés et comptabilisés.
- Un PID réutilisé est détecté en comparant le nom courant à l'inventaire de référence.

## 📁 4. Rangement des Fichiers du Bureau (`desktop`)

> [!NOTE]
> Le rangement classe les fichiers et raccourcis dans des dossiers thématiques. Il ne déplace pas les coordonnées des icônes à l'écran.

### Workflow en 4 étapes

```cmd
# 1. Scanner les fichiers éligibles du Bureau
deskctl desktop scan --out desktop-files.json

# 2. Établir le plan de rangement selon les règles
deskctl desktop plan --input desktop-files.json --rules rules.json --out desktop-plan.json

# 3. Simuler le rangement (mode sans risque)
deskctl desktop apply --plan desktop-plan.json

# 4. Appliquer réellement avec journal d'annulation
deskctl desktop apply --plan desktop-plan.json --execute --undo-log undo-log.json

# 5. Annuler si nécessaire (simulation par défaut)
deskctl desktop undo --log undo-log.json
deskctl desktop undo --log undo-log.json --execute
```

### Règles de classement (`rules.json`)

- **Documents** : `.pdf`, `.docx`, `.doc`, `.xlsx`, `.xls`, `.pptx`, `.txt`, `.md`, `.csv`, `.odt`
- **Images** : `.png`, `.jpg`, `.jpeg`, `.gif`, `.svg`, `.bmp`, `.webp`, `.ico`
- **Archives** : `.zip`, `.tar`, `.gz`, `.7z`, `.rar`
- **Installateurs** : `.exe`, `.msi`, `.iso`, `.dmg`
- **Raccourcis** : `.lnk`, `.url`
- **Autres** : catégorie de repli pour les extensions non spécifiées.

### Protections obligatoires & OneDrive

1. **Détection dynamique du Bureau réel** : résolution via l'API Shell Win32 `SHGetKnownFolderPath` (`FOLDERID_Desktop`) avec drapeaux non-bloquants et repli sur le Registre (`User Shell Folders`).
2. **Avertissement OneDrive** : si le Bureau est synchronisé (ex: `OneDrive - Entreprise\Bureau`), un avertissement signale que les déplacements seront synchronisés sur le cloud.
3. **Zéro effacement / Zéro écrasement** : aucun fichier n'est supprimé. En cas de collision (un fichier de même nom existe déjà dans le dossier cible), le fichier reste intact sur le Bureau et un conflit est consigné.
4. **Fichiers protégés & ignorés** :
   - `desktop.ini` ignoré.
   - Fichiers cachés et système ignorés.
   - Répertoires existants non déplacés.
   - Liens symboliques et jonctions NTFS non traversés (`os.Lstat`).
5. **Préservation des raccourcis** : les fichiers `.lnk` et `.url` sont déplacés en tant que fichiers propres ; leurs cibles ne sont jamais modifiées.
6. **Contrôle d'évasion** : interdiction stricte de déplacer un fichier en dehors de l'arborescence du Bureau utilisateur.
7. **Fichiers modifiés ou disparus** : si la date de modification ou la taille a changé entre le scan et l'application, le fichier est ignoré par prudence.
8. **Bureau public préservé** : `C:\Users\Public\Desktop` n'est jamais altéré.

## 🧪 5. Tests Automatisés

Pour lancer la suite complète des tests unitaires et de simulation :

```cmd
cd tools/deskctl
npm --prefix frontend ci
npm --prefix frontend run build
npm --prefix frontend test
go test -v ./...
```

Le build frontend est nécessaire avant `go test ./...` pour les assets embarqués
de la GUI. Les tests Go ciblant uniquement `internal` ne nécessitent pas ces assets.
Les tests des nouveaux moteurs et de la GUI simulent les déplacements : ils ne
modifient pas les bureaux réels.

La suite teste :

- La navigation TUI, la confirmation obligatoire et le rendu sur terminal étroit.
- Les parcours de simulation, rangement et annulation sur un Bureau temporaire.
- L'énumération COM de plusieurs fenêtres par PID sur différents bureaux et la validation des HWND.
- La validation des GUID, le rejet des anciens layouts nommés et la protection PID.
- L'énumération COM des bureaux et la lecture de leurs noms.
- La simulation du moteur de placement via un mover de test, sans déplacer de fenêtre.
- La détection des collisions de fichiers et la protection contre l'évasion de dossier.

## ⚙️ Compilation

```cmd
cd tools/deskctl
go build -o deskctl.exe main.go
```
