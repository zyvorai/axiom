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

| English | Deutsch | Español | Français | Português | Italiano | 日本語 | 简体中文 |
|---|---|---|---|---|---|---|---|
| Add | Hinzufügen | Añadir | Ajouter | Adicionar | Aggiungi | 追加 | 添加 |
| Apply | Anwenden | Aplicar | Appliquer | Aplicar | Applica | 適用 | 应用 |
| Cancel | Abbrechen | Cancelar | Annuler | Cancelar | Annulla | キャンセル | 取消 |
| Close | Schließen | Cerrar | Fermer | Fechar | Chiudi | 閉じる | 关闭 |
| Continue | Weiter | Continuar | Continuer | Continuar | Continua | 続行 | 继续 |
| Create | Erstellen | Crear | Créer | Criar | Crea | 作成 | 创建 |
| Delete | Löschen | Eliminar | Supprimer | Excluir | Elimina | 削除 | 删除 |
| Remove | Entfernen | Quitar | Retirer | Remover | Rimuovi | 除去 | 移除 |
| Edit | Bearbeiten | Editar | Modifier | Editar | Modifica | 編集 | 编辑 |
| Save | Speichern | Guardar | Enregistrer | Salvar | Salva | 保存 | 保存 |
| Search | Suchen | Buscar | Rechercher | Pesquisar | Cerca | 検索 | 搜索 |
| Refresh | Aktualisieren | Actualizar | Actualiser | Atualizar | Aggiorna | 更新 | 刷新 |
| Retry / Try again | Erneut versuchen | Reintentar | Réessayer | Tentar novamente | Riprova | 再試行 | 重试 |
| Settings | Einstellungen | Ajustes | Paramètres | Configurações | Impostazioni | 設定 | 设置 |
| Sign in / Sign out | Anmelden / Abmelden | Iniciar sesión / Cerrar sesión | Se connecter / Se déconnecter | Entrar / Sair | Accedi / Esci | サインイン / サインアウト | 登录 / 退出登录 |
| Username / Password | Benutzername / Passwort | Usuario / Contraseña | Nom d'utilisateur / Mot de passe | Nome de usuário / Senha | Nome utente / Password | ユーザー名 / パスワード | 用户名 / 密码 |
| User | Benutzer | Usuario | Utilisateur | Usuário | Utente | ユーザー | 用户 |
| Viewer (role) | Betrachter | Lector | Lecteur | Leitor | Lettore | 閲覧者 | 查看者 |
| Operator (role) | Operator | Operador | Opérateur | Operador | Operatore | オペレーター | 操作员 |
| Admin (role) | Admin | Administrador | Administrateur | Administrador | Amministratore | 管理者 | 管理员 |
| Language | Sprache | Idioma | Langue | Idioma | Lingua | 言語 | 语言 |
| Theme | Design | Tema | Thème | Tema | Tema | テーマ | 主题 |
| Light / Dark / System | Hell / Dunkel / System | Claro / Oscuro / Sistema | Clair / Sombre / Système | Claro / Escuro / Sistema | Chiaro / Scuro / Sistema | ライト / ダーク / システム | 浅色 / 深色 / 跟随系统 |
| Overview | Übersicht | Resumen | Vue d'ensemble | Visão geral | Panoramica | 概要 | 概览 |
| Workloads | Workloads | Cargas de trabajo | Charges de travail | Cargas de trabalho | Carichi di lavoro | ワークロード | 工作负载 |
| Cluster | Cluster | Clúster | Cluster | Cluster | Cluster | クラスター | 集群 |
| Fleet | Flotte | Flota | Flotte | Frota | Flotta | フリート | 集群舰队 |
| Events | Ereignisse | Eventos | Événements | Eventos | Eventi | イベント | 事件 |
| Logs | Logs | Registros | Journaux | Logs | Log | ログ | 日志 |
| Alerts | Warnungen | Alertas | Alertes | Alertas | Avvisi | アラート | 告警 |
| Timeline | Zeitachse | Cronología | Chronologie | Linha do tempo | Cronologia | タイムライン | 时间线 |
| Health check | Zustandsprüfung | Comprobación de estado | Bilan de santé | Verificação de integridade | Controllo di integrità | ヘルスチェック | 健康检查 |
| Security | Sicherheit | Seguridad | Sécurité | Segurança | Sicurezza | セキュリティ | 安全 |
| Storage | Speicher | Almacenamiento | Stockage | Armazenamento | Archiviazione | ストレージ | 存储 |
| Networking | Netzwerk | Redes | Réseau | Rede | Rete | ネットワーク | 网络 |
| Backups | Backups | Copias de seguridad | Sauvegardes | Backups | Backup | バックアップ | 备份 |
| Snapshot | Snapshot | Instantánea | Instantané | Snapshot | Snapshot | スナップショット | 快照 |
| Quotas & Limits | Quotas & Limits | Cuotas y límites | Quotas et limites | Cotas e limites | Quote e limiti | クォータと制限 | 配额与限制 |
| Autoscaling | Autoscaling | Escalado automático | Mise à l'échelle auto | Escalonamento automático | Scalabilità automatica | オートスケーリング | 自动扩缩容 |
| Autoscaler | Autoscaler | Autoescalador | Autoscaler | Autoescalador | Autoscaler | オートスケーラー | 自动扩缩器 |
| Cost / Budget | Kosten / Budget | Coste / Presupuesto | Coût / Budget | Custo / Orçamento | Costo / Budget | コスト / 予算 | 成本 / 预算 |
| Virtual machine | Virtuelle Maschine | Máquina virtual | Machine virtuelle | Máquina virtual | Macchina virtuale | 仮想マシン | 虚拟机 |
| Custom resources | Custom Resources | Recursos personalizados | Ressources personnalisées | Recursos personalizados | Risorse personalizzate | カスタムリソース | 自定义资源 |
| Healthy | Fehlerfrei | Correcto | Sain | Íntegro | Integro | 正常 | 健康 |
| Degraded | Beeinträchtigt | Degradado | Dégradé | Degradado | Degradato | 低下 | 降级 |
| Unreachable | Nicht erreichbar | Inaccesible | Injoignable | Inacessível | Non raggiungibile | 到達不能 | 不可达 |
| Running | Läuft | En ejecución | En cours | Em execução | In esecuzione | 実行中 | 运行中 |
| Pending | Ausstehend | Pendiente | En attente | Pendente | In attesa | 保留中 | 等待中 |
| Failed | Fehlgeschlagen | Fallido | Échec | Falhou | Non riuscito | 失敗 | 失败 |
| Warning | Warnung | Advertencia | Avertissement | Aviso | Avviso | 警告 | 警告 |
| Critical | Kritisch | Crítico | Critique | Crítico | Critico | 重大 | 严重 |
| Ready | Bereit | Listo | Prêt | Pronto | Pronto | 準備完了 | 就绪 |
| Age | Alter | Antigüedad | Âge | Idade | Età | 経過時間 | 存在时间 |
| Restarts | Neustarts | Reinicios | Redémarrages | Reinicializações | Riavvii | 再起動回数 | 重启次数 |
| Replicas | Replikas | Réplicas | Réplicas | Réplicas | Repliche | レプリカ | 副本数 |
| Memory | Arbeitsspeicher | Memoria | Mémoire | Memória | Memoria | メモリー | 内存 |
| Image (container) | Image | Imagen | Image | Imagem | Immagine | イメージ | 镜像 |
| Cordon / Uncordon | Sperren / Entsperren | Acordonar / Desacordonar | Isoler / Réintégrer | Isolar / Reintegrar | Cordon / Uncordon | Cordon / Uncordon | 封锁 / 解除封锁 |
| Drain | Leeren (Drain) | Drenar | Drainer | Drenar | Drain | ドレイン | 排空 |
| Scale | Skalieren | Escalar | Mettre à l'échelle | Escalar | Scala | スケール | 扩缩 |
| Restart | Neu starten | Reiniciar | Redémarrer | Reiniciar | Riavvia | 再起動 | 重启 |
| Rollback / Roll back | Zurücksetzen | Revertir | Restaurer | Reverter | Rollback | ロールバック | 回滚 |
| Install / Uninstall | Installieren / Deinstallieren | Instalar / Desinstalar | Installer / Désinstaller | Instalar / Desinstalar | Installa / Disinstalla | インストール / アンインストール | 安装 / 卸载 |
| Release (Helm) | Release | Release | Release | Release | Release | リリース | Release |
| Finding | Befund | Hallazgo | Constat | Constatação | Rilevamento | 検出結果 | 发现项 |
| Rightsizing | Rightsizing | Ajuste de recursos | Dimensionnement | Ajuste de recursos | Rightsizing | ライトサイジング | 资源优化 |
| Request / Limit (resources) | Request / Limit | Request / Limit | Request / Limit | Request / Limit | Request / Limit | Request / Limit | Request / Limit |
| Two-factor authentication (MFA) | Zwei-Faktor-Authentifizierung | Autenticación de dos factores | Authentification à deux facteurs | Autenticação de dois fatores | Autenticazione a due fattori | 2 要素認証 | 双因素认证 |
| Recovery code | Wiederherstellungscode | Código de recuperación | Code de récupération | Código de recuperação | Codice di recupero | リカバリーコード | 恢复码 |
| Single sign-on (SSO) | Single Sign-on | Inicio de sesión único | Authentification unique | Logon único | Single sign-on | シングルサインオン | 单点登录 |
| Read-only | Schreibgeschützt | Solo lectura | Lecture seule | Somente leitura | Sola lettura | 読み取り専用 | 只读 |
| Dry run | Probelauf | Simulación | Simulation | Simulação | Prova (dry run) | ドライラン | 试运行 |

## Style

- German: formal "Sie"; nouns capitalised; prefer short imperative button
  labels ("Speichern", not "Jetzt speichern").
- Spanish: "tú" form is avoided in favour of neutral/impersonal phrasing;
  buttons are infinitives ("Guardar").
- French: "vous"; buttons are infinitives ("Enregistrer"); a non-breaking
  space is not required before `:`/`?` in UI strings.
- Portuguese: Brazilian Portuguese (`pt` is formatted as pt-BR by `Intl`);
  buttons are infinitives ("Salvar", "Excluir").
- Italian: impersonal/formal-neutral phrasing; buttons use the imperative as
  Italian software does ("Salva", "Elimina", "Annulla").
- Japanese: polite です/ます in sentences, short noun labels on buttons
  ("保存", "削除"); full-width 。、（） in Japanese text. Only the `_other`
  plural form exists (no `_one` keys).
- Simplified Chinese: mainland terms (集群, 工作负载); full-width ，。：（） in
  Chinese text. Only the `_other` plural form exists (no `_one` keys).
- Use the typographic ellipsis "…" (not "...") everywhere, as English does.
