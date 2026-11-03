# Translation glossary

Keep terminology consistent across the UI. When a term is not here, pick the
word the language's Kubernetes documentation uses, then add it below.

## Never translated (all languages)

Kubernetes kinds and API terms stay in English, capitalised as Kubernetes
writes them: Pod, Node, Namespace, Deployment, StatefulSet, DaemonSet,
ReplicaSet, Job, CronJob, Service, Ingress, ConfigMap, Secret,
PersistentVolumeClaim (PVC), StorageClass, NetworkPolicy, HPA, CRD, RBAC,
Role, ClusterRole, RoleBinding, ServiceAccount, Helm, chart, GitOps, Argo CD,
Flux, Cilium, Hubble, KubeVirt, Velero, Prometheus, label, annotation,
taint, toleration, kubeconfig, `kubectl …` commands, YAML/JSON, field names
(`metadata.name`), metric names, event reasons (`BackOff`, `CrashLoopBackOff`),
and product names (Axiom, Zyvor, Atlas, PacketWolf). Plurals follow English
too: "3 Pods", "3 Pods" (de), "3 pods" (es/fr lower-case in running text is fine).

Text that comes from the API (error messages, event messages, findings) is
shown exactly as the server sent it.

## UI chrome

| English | Deutsch | Español | Français |
|---|---|---|---|
| Add | Hinzufügen | Añadir | Ajouter |
| Apply | Anwenden | Aplicar | Appliquer |
| Cancel | Abbrechen | Cancelar | Annuler |
| Close | Schließen | Cerrar | Fermer |
| Continue | Weiter | Continuar | Continuer |
| Create | Erstellen | Crear | Créer |
| Delete | Löschen | Eliminar | Supprimer |
| Remove | Entfernen | Quitar | Retirer |
| Edit | Bearbeiten | Editar | Modifier |
| Save | Speichern | Guardar | Enregistrer |
| Search | Suchen | Buscar | Rechercher |
| Refresh | Aktualisieren | Actualizar | Actualiser |
| Retry / Try again | Erneut versuchen | Reintentar | Réessayer |
| Settings | Einstellungen | Ajustes | Paramètres |
| Sign in / Sign out | Anmelden / Abmelden | Iniciar sesión / Cerrar sesión | Se connecter / Se déconnecter |
| Username / Password | Benutzername / Passwort | Usuario / Contraseña | Nom d'utilisateur / Mot de passe |
| User | Benutzer | Usuario | Utilisateur |
| Viewer (role) | Betrachter | Lector | Lecteur |
| Operator (role) | Operator | Operador | Opérateur |
| Admin (role) | Admin | Administrador | Administrateur |
| Language | Sprache | Idioma | Langue |
| Theme | Design | Tema | Thème |
| Light / Dark / System | Hell / Dunkel / System | Claro / Oscuro / Sistema | Clair / Sombre / Système |
| Overview | Übersicht | Resumen | Vue d'ensemble |
| Workloads | Workloads | Cargas de trabajo | Charges de travail |
| Cluster | Cluster | Clúster | Cluster |
| Fleet | Flotte | Flota | Flotte |
| Events | Ereignisse | Eventos | Événements |
| Logs | Logs | Registros | Journaux |
| Alerts | Warnungen | Alertas | Alertes |
| Timeline | Zeitachse | Cronología | Chronologie |
| Health check | Zustandsprüfung | Comprobación de estado | Bilan de santé |
| Security | Sicherheit | Seguridad | Sécurité |
| Storage | Speicher | Almacenamiento | Stockage |
| Networking | Netzwerk | Redes | Réseau |
| Backups | Backups | Copias de seguridad | Sauvegardes |
| Snapshot | Snapshot | Instantánea | Instantané |
| Quotas & Limits | Quotas & Limits | Cuotas y límites | Quotas et limites |
| Autoscaling | Autoscaling | Escalado automático | Mise à l'échelle auto |
| Autoscaler | Autoscaler | Autoescalador | Autoscaler |
| Cost / Budget | Kosten / Budget | Coste / Presupuesto | Coût / Budget |
| Virtual machine | Virtuelle Maschine | Máquina virtual | Machine virtuelle |
| Custom resources | Custom Resources | Recursos personalizados | Ressources personnalisées |
| Healthy | Fehlerfrei | Correcto | Sain |
| Degraded | Beeinträchtigt | Degradado | Dégradé |
| Unreachable | Nicht erreichbar | Inaccesible | Injoignable |
| Running | Läuft | En ejecución | En cours |
| Pending | Ausstehend | Pendiente | En attente |
| Failed | Fehlgeschlagen | Fallido | Échec |
| Warning | Warnung | Advertencia | Avertissement |
| Critical | Kritisch | Crítico | Critique |
| Ready | Bereit | Listo | Prêt |
| Age | Alter | Antigüedad | Âge |
| Restarts | Neustarts | Reinicios | Redémarrages |
| Replicas | Replikas | Réplicas | Réplicas |
| Memory | Arbeitsspeicher | Memoria | Mémoire |
| Image (container) | Image | Imagen | Image |
| Cordon / Uncordon | Sperren / Entsperren | Acordonar / Desacordonar | Isoler / Réintégrer |
| Drain | Leeren (Drain) | Drenar | Drainer |
| Scale | Skalieren | Escalar | Mettre à l'échelle |
| Restart | Neu starten | Reiniciar | Redémarrer |
| Rollback / Roll back | Zurücksetzen | Revertir | Restaurer |
| Install / Uninstall | Installieren / Deinstallieren | Instalar / Desinstalar | Installer / Désinstaller |
| Release (Helm) | Release | Release | Release |
| Finding | Befund | Hallazgo | Constat |
| Rightsizing | Rightsizing | Ajuste de recursos | Dimensionnement |
| Request / Limit (resources) | Request / Limit | Request / Limit | Request / Limit |
| Two-factor authentication (MFA) | Zwei-Faktor-Authentifizierung | Autenticación de dos factores | Authentification à deux facteurs |
| Recovery code | Wiederherstellungscode | Código de recuperación | Code de récupération |
| Single sign-on (SSO) | Single Sign-on | Inicio de sesión único | Authentification unique |
| Read-only | Schreibgeschützt | Solo lectura | Lecture seule |
| Dry run | Probelauf | Simulación | Simulation |

## Style

- German: formal "Sie"; nouns capitalised; prefer short imperative button
  labels ("Speichern", not "Jetzt speichern").
- Spanish: "tú" form is avoided in favour of neutral/impersonal phrasing;
  buttons are infinitives ("Guardar").
- French: "vous"; buttons are infinitives ("Enregistrer"); a non-breaking
  space is not required before `:`/`?` in UI strings.
- Use the typographic ellipsis "…" (not "...") everywhere, as English does.
