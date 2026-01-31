import { useState } from "react";
import styles from "./Payment.module.css";

type Step = "form" | "processing" | "success" | "error";

const Payment: React.FC = () => {
  const [amount, setAmount] = useState("");
  const [description, setDescription] = useState("");
  const [step, setStep] = useState<Step>("form");

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!amount || Number(amount) <= 0) return;
    setStep("processing");
    // mock
    setTimeout(() => setStep("success"), 1500);
  };

  const reset = () => {
    setAmount("");
    setDescription("");
    setStep("form");
  };

  if (step === "processing") {
    return (
      <div className={styles.wrapper}>
        <div className={styles.card} aria-busy="true">
          <div className={styles.spinner} aria-hidden />
          <p className={styles.processingText}>Processing payment…</p>
          <p className={styles.processingHint}>
            Routing through available provider
          </p>
        </div>
      </div>
    );
  }

  if (step === "success") {
    return (
      <div className={styles.wrapper}>
        <div className={`${styles.card} ${styles.successCard}`}>
          <div className={styles.successIcon} aria-hidden>
            ✓
          </div>
          <h2 className={styles.successTitle}>Payment successful</h2>
          <p className={styles.successAmount}>
            {amount && (
              <>
                {new Intl.NumberFormat("en-IN", {
                  style: "currency",
                  currency: "INR",
                }).format(Number(amount))}
              </>
            )}
          </p>
          {description && (
            <p className={styles.successDescription}>{description}</p>
          )}
          <button type="button" className={styles.button} onClick={reset}>
            Pay again
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.wrapper}>
      <div className={styles.card}>
        <h1 className={styles.title}>Pay Securely</h1>
        <p className={styles.subtitle}>
          Enter amount and pay. We route your payment securely.
        </p>

        <form onSubmit={handleSubmit} className={styles.form}>
          <label className={styles.label} htmlFor="amount">
            Amount
          </label>
          <div className={styles.amountRow}>
            <span className={styles.currency}>₹</span>
            <input
              id="amount"
              type="number"
              inputMode="decimal"
              min="1"
              step="0.01"
              placeholder="0.00"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              className={styles.input}
              required
            />
          </div>

          <button type="submit" className={styles.button} disabled={!amount}>
            Pay
          </button>
        </form>

        <p className={styles.trust}>
          Secured by Vortex · routed via best-available provider
        </p>
      </div>
    </div>
  );
};

export default Payment;
