package ui

// helpContent est le texte affiché par l'écran d'aide (F1), rappelant le
// fonctionnement des deux panneaux et les raccourcis clavier. Voir
// SPEC.md §3.1 et §4 — à tenir synchronisé avec la table des raccourcis.
const helpContent = `[yellow]TermDevTools[white] — client Elasticsearch en mode terminal

[green]Panneau gauche[white] : requêtes "MÉTHODE endpoint" + JSON optionnel
sur les lignes suivantes. Lignes [gray]#[white] = commentaires.

[green]Panneau droit[white] : résultat de la dernière requête — JSON coloré,
ou texte brut (ex. réponses _cat/*).

[yellow]Raccourcis clavier[white]

  [aqua]Ctrl+Entrée[white]      Exécuter la requête sous le curseur
  [aqua]Tab[white]              Compléter un endpoint en cours de frappe
  [aqua]Ctrl+←/→[white]         Changer de panneau
  [aqua]Ctrl+Maj+←/→[white]     Redimensionner le split gauche/droite
  [aqua]Ctrl+F[white]           Rechercher dans le panneau actif
  [aqua]Ctrl+S[white]           Sauvegarder (gauche) / exporter (droite)
  [aqua]F2[white]               Copier le résultat (panneau droit) dans le presse-papier
  [aqua]F1[white]               Afficher cette aide
  [aqua]Ctrl+C[white]           Quitter (sauvegarde automatiquement le panneau gauche)

[yellow]Fichiers[white]

  [aqua]~/.config/termdevtools/[white]  (personnel)
    config.yaml     clusters connus
    queries_*.txt   sauvegarde par cluster (Ctrl+S)
  [aqua]<dossier du binaire>/[white]  (équipe, partagé)
    cheatsheet.txt  requêtes par défaut de l'éditeur
    endpoints.txt   liste proposée par la complétion Tab
    exports/        résultats exportés (Ctrl+S)

[gray]Echap pour fermer cette aide.[white]`
