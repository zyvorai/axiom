> **DRAFT — NOT LEGAL ADVICE.** This is a starting-point template for
> Zyvor's counsel to review before use. Every bracketed `[...]`
> placeholder needs a real value or removal.
>
> **Worth confirming with counsel first:** because Axiom is self-hosted
> — it runs entirely inside the customer's own Kubernetes cluster, and
> Zyvor never receives, stores, or has access to the customer's data or
> infrastructure — a full GDPR-style Data Processing Agreement may not
> be necessary at all (Zyvor may not qualify as a "processor" in the
> relevant sense). This draft exists in case a customer's procurement
> process asks for one anyway, or a future SaaS/hosted offering needs
> one. Have counsel confirm applicability before offering this to a
> customer.

# Data Processing Agreement (DPA)

Between **[ZYVOR LEGAL ENTITY NAME]** ("Zyvor") and Customer, entered
into as an addendum to the End User License Agreement (see
`legal/EULA-DRAFT.md`).

## 1. Scope

This DPA applies only to the extent Zyvor processes personal data on
Customer's behalf in connection with the Software. Given Axiom's
self-hosted deployment model, Zyvor typically processes no Customer
personal data — Customer's cluster, its workloads, and any data
Customer's operators view through Axiom's UI all remain within
Customer's own infrastructure at all times. [COUNSEL: confirm whether
any incidental telemetry, crash reports, or support-ticket attachments
Customer might send Zyvor bring this DPA into effect, and scope
accordingly.]

## 2. Definitions

Uses the definitions of "personal data," "processing," "controller,"
and "processor" from [APPLICABLE LAW — e.g., GDPR Art. 4, or a
jurisdiction-appropriate equivalent].

## 3. Processor Obligations

To the extent Zyvor does process personal data on Customer's behalf,
Zyvor will:

- process it only on Customer's documented instructions;
- ensure personnel with access are bound by confidentiality obligations;
- implement appropriate technical and organizational security measures
  [REFERENCE ZYVOR'S SECURITY PROGRAM / SOC2 REPORT ONCE AVAILABLE —
  see `docs/COMPLIANCE.md` for current status];
- assist Customer in responding to data subject requests, to the
  extent Zyvor holds relevant data;
- notify Customer without undue delay after becoming aware of a
  personal data breach affecting Customer's data; and
- delete or return all personal data at the end of the engagement,
  except as law requires retention.

## 4. Sub-processors

[LIST ANY THIRD-PARTY SUB-PROCESSORS ZYVOR USES THAT WOULD TOUCH
CUSTOMER DATA — e.g., none, if Axiom is purely self-hosted with no
managed backend. Confirm and either populate or state "None" here.]

## 5. International Transfers

[ONLY RELEVANT IF ZYVOR PROCESSES DATA OUTSIDE CUSTOMER'S REGION —
STANDARD CONTRACTUAL CLAUSES OR EQUIVALENT MECHANISM, IF APPLICABLE.]

## 6. Audit Rights

[STANDARD AUDIT CLAUSE — e.g., Customer may request evidence of
compliance (such as a SOC2 report) no more than once per year, subject
to reasonable notice.]

## 7. Liability

Liability under this DPA is governed by the limitation of liability
clause in the underlying EULA, unless applicable law requires
otherwise.

## 8. Term

This DPA remains in effect for as long as the underlying EULA is in
effect and Zyvor processes personal data on Customer's behalf under it.
