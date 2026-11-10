# Plan d'implementation : groupes de fenetres

## Objectif

Creer des groupes nommes independants des regles. Retrouver leurs fenetres par
criteres application/titre, les disposer en tuiles H/V dans une zone configurable,
puis deplacer le groupe vers un ecran et un bureau, ou tous les bureaux.

## Decisions validees

- Plusieurs regles peuvent referencer le meme groupe sans copier sa disposition.
- Le groupe conserve ses membres, son arbre H/V et sa zone relative par defaut.
- Chaque regle definit ecran/bureau et peut remplacer cette zone.
- Les manipulations dans deskctl preparent un lot ; elles ne modifient pas les
  regles sans enregistrement explicite et ne deplacent rien avant application.
- Le suivi des gestes Windows est optionnel, active par groupe pendant la session.
  Aucun suivi ni mouvement automatique au demarrage.
- Au relachement d'une fenetre deplacee, les autres membres recoivent la meme
  translation, sans redimensionnement.
- Le redimensionnement d'une vraie fenetre et les changements de bureau effectues
  dans Windows ne sont pas suivis.
- Des regles actives contradictoires ou des appartenances ambigues produisent
  des conflits explicites.
- Les applications rouvertes retrouvent leur appartenance, mais les regles restent
  appliquees uniquement sur demande.

## Etat initial et reutilisation

Le projet possede deja des arbres de tuiles H/V imbriques, des proportions,
un editeur React, un moteur de placement natif et une integration FancyWM.
Les groupes sont initialement integres aux regles v2 : il manque un catalogue
partage, une zone configurable et le suivi des vrais gestes Windows.

Reutiliser `internal/windowrules`, `internal/layout`, `internal/windoworganizer`,
`internal/tiling`, `internal/windowstore` et les composants du Studio.
Ne pas construire un deuxieme moteur de disposition.

## Etapes

1. **Modele et stockage v3.** Ajouter `groups`, `groupId` et les zones relatives.
   Valider les references et les rectangles. Migrer v1/v2 avec sauvegarde verifiee,
   ecriture atomique et identifiants stables. Importer/exporter groupes et regles
   ensemble. Interdire la suppression d'un groupe reference.
2. **Planification groupee.** Resoudre les membres, dedupliquer les regles
   equivalentes, bloquer les contradictions et preparer une affectation complete
   avec une seule revision du plan.
3. **Execution et recuperation.** Deplacer les bureaux puis calculer les tuiles
   dans la zone. Revalider les identites des fenetres. Journaliser les positions
   avant/apres et rendre les echecs partiels visibles.
4. **Gestionnaire de groupes.** Ajouter creation, capture de selection, edition,
   proportions, zone et association a une regle. Conserver les regles simples.
5. **Canvas.** Afficher des cadres de groupe deplacables et redimensionnables,
   avec destinations ecran/bureau et commandes accessibles. Les gestes restent
   des preparations de lot.
6. **Suivi Windows.** Ajouter hooks Win32, traitement serialise, controle de
   reentrance, activation par groupe, notifications Wails et arret propre.
7. **Validation et documentation.** Couvrir migration, zones, conflits, gestes,
   journaux, interactions React et compilation GUI.

Dependances : 2 depend de 1 ; 3 de 2 ; 4 de 3 ; 5 de 4 ; 6 de 3 et 4 ;
7 de 5 et 6.

## Garde-fous et limites

- Aucun service Windows ni suivi apres fermeture de deskctl.
- Les zones arbitraires utilisent le moteur natif ; FancyWM reste disponible
  pour les dispositions plein ecran compatibles.
- Le suivi des gestes Windows est refuse pendant que FancyWM fonctionne, pour
  eviter deux gestionnaires concurrents.
- Le groupe suivi doit rester coherent : meme bureau ou membres tous epingles.
  Les tailles restent conservees ; une translation non admissible est signalee.
- Les callbacks Win32 ne deplacent pas directement les fenetres.
- Aucun mouvement supplementaire si le journal ne peut pas etre sauvegarde.
- Un journal incertain bloque son remplacement. L'annulation revalide identite
  et position ; elle ne restaure pas aveuglement une fenetre modifiee.
- L'operation est atomique pour la preparation, pas transactionnelle dans Windows.
- Retour a une ancienne version : conserver l'executable precedent et restaurer
  sa sauvegarde JSON ; l'ancien lecteur ne comprend pas le format v3.

## Preuves attendues

Tests Go : migration idempotente et sauvegarde, references partagees, zones H/V,
moniteurs a coordonnees negatives, destination absente, groupe bloque, revision
unique, journal inaccessible, suivi simule et hooks installes puis retires.

Tests React : edition et reutilisation d'un groupe, conservation des groupes
pendant modification des regles, depot ecran/bureau, redimensionnement, suivi
optionnel et invalidation d'un plan apres geste Windows.

Executer les tests cibles, le build TypeScript/Vite et la compilation Go GUI.
Ne pas deplacer les fenetres de travail pour une verification automatique.
