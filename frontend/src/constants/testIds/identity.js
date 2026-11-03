// Test IDs for account security: the two-factor step on the sign-in page, the
// "Account security" dialog opened from the user menu, and the MFA / session
// controls on the Users page. Naming follows ./auth.js.

export const MFA = {
    // Sign-in page
    loginCode: 'login-mfa-code',
    loginSubmit: 'login-mfa-submit',
    loginUseRecovery: 'login-mfa-use-recovery',
    loginUseApp: 'login-mfa-use-app',
    // User menu + dialog
    menuItem: 'user-menu-account-security',
    dialog: 'account-security-dialog',
    status: 'account-security-status',
    setupPassword: 'account-security-setup-password',
    setupStart: 'account-security-setup-start',
    secret: 'account-security-secret',
    qr: 'account-security-qr',
    gate: 'mfa-enrollment-gate',
    gateOpen: 'mfa-enrollment-gate-open',
    gateSignOut: 'mfa-enrollment-gate-signout',
    copySecret: 'account-security-copy-secret',
    enableCode: 'account-security-enable-code',
    enableSubmit: 'account-security-enable-submit',
    recoveryCodes: 'account-security-recovery-codes',
    recoveryDone: 'account-security-recovery-done',
    disablePassword: 'account-security-disable-password',
    disableCode: 'account-security-disable-code',
    disableSubmit: 'account-security-disable-submit',
    signOutAll: 'account-security-signout-all',
    // Users page
    usersBadge: (u) => `user-mfa-${u}`,
    usersReset: (u) => `user-mfa-reset-${u}`,
    usersRevoke: (u) => `user-revoke-sessions-${u}`,
};
