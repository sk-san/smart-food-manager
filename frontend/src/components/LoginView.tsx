import React from 'react';
import { Code, FlaskConical, Leaf } from 'lucide-react';
import { GuestRole } from '../types/nutrition';

interface LoginViewProps {
  /** Purely presentational — fired by the Sign in button, no real auth. */
  onSignIn: () => void;
  /** Auth bypass for testing: enters the app as the given guest role. */
  onGuestLogin: (role: GuestRole) => void;
}

// Template sign-in screen (design 2c): brand panel on the left, credential
// form on the right. There is no authentication behind it yet — the backend
// JWT flow is a follow-up — so submitting simply returns to the app.
const LoginView: React.FC<LoginViewProps> = ({ onSignIn, onGuestLogin }) => {
  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSignIn();
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-bg p-4 md:p-8">
      <div className="animate-in fade-in grid w-full max-w-4xl overflow-hidden rounded-card bg-surface shadow-lg duration-500 md:min-h-[560px] md:grid-cols-[1fr_1.1fr]">
        {/* Brand panel */}
        <div className="flex flex-col bg-accent-2-200 p-9">
          <div className="flex items-center gap-2.5">
            <div className="grid h-9 w-9 place-items-center rounded-full bg-neutral-100">
              <Leaf size={18} strokeWidth={2.75} className="text-accent-2-800" />
            </div>
            <span className="font-display text-[19px] text-accent-2-900">Nutri</span>
          </div>

          <h2 className="mt-7 max-w-[9ch] text-[28px] leading-[1.1] text-accent-2-900 md:text-[34px]">
            Welcome back to the table.
          </h2>
          <p className="mt-3 max-w-[26ch] text-sm leading-relaxed text-accent-2-800">
            Your log, your larder and Nutri are right where you left them.
          </p>

          {/* Nutri, waiting patiently (the design-system blob illustration) */}
          <svg
            viewBox="0 0 100 100"
            className="mt-auto hidden h-40 w-40 self-end md:block"
            aria-hidden="true"
          >
            <ellipse cx="50" cy="90" rx="28" ry="6" fill="var(--color-accent-2-400)" opacity="0.5" />
            <path
              d="M50 14 Q50 2 38 6 Q49 9 50 14 M50 14 Q51 4 61 7 Q52 10 50 14"
              fill="var(--color-accent-2-700)"
            />
            <path
              d="M22 55 a28 26 0 1 0 56 0 a28 26 0 1 0 -56 0"
              fill="var(--color-neutral-100)"
              stroke="var(--color-accent-2-600)"
              strokeWidth="2.5"
            />
            <circle cx="41" cy="52" r="3.4" fill="var(--color-neutral-900)" />
            <circle cx="59" cy="52" r="3.4" fill="var(--color-neutral-900)" />
            <path
              d="M43 62 Q50 68 57 62"
              fill="none"
              stroke="var(--color-neutral-900)"
              strokeWidth="2.75"
              strokeLinecap="round"
            />
            <circle cx="33" cy="60" r="4.5" fill="var(--color-accent-300)" opacity="0.85" />
            <circle cx="67" cy="60" r="4.5" fill="var(--color-accent-300)" opacity="0.85" />
          </svg>
        </div>

        {/* Sign-in form */}
        <form onSubmit={handleSubmit} className="flex flex-col justify-center p-8 md:p-12">
          <h3 className="mb-5 text-[25px] text-ink">Sign in</h3>

          <div className="mb-4">
            <label className="field-label" htmlFor="login-email">
              Email
            </label>
            <input
              id="login-email"
              type="email"
              autoComplete="email"
              placeholder="me@example.com"
              className="input"
            />
          </div>

          <div className="mb-2">
            <label className="field-label" htmlFor="login-password">
              Password
            </label>
            <input
              id="login-password"
              type="password"
              autoComplete="current-password"
              placeholder="••••••••"
              className="input"
            />
          </div>

          <a href="#" onClick={(e) => e.preventDefault()} className="self-end text-[12.5px] text-accent-700">
            Forgot password?
          </a>

          <button type="submit" className="btn btn-primary mt-5 w-full py-3 text-[15px] shadow-sm">
            Sign in
          </button>

          <div className="my-4 flex items-center gap-3 text-xs text-neutral-500">
            <span className="h-px flex-1 bg-divider" />
            new here?
            <span className="h-px flex-1 bg-divider" />
          </div>

          <button type="button" className="btn btn-secondary w-full py-3">
            Create an account
          </button>

          {/* Auth bypass for testing — no credentials, straight into the app. */}
          <div className="kicker mb-2 mt-6 text-center text-neutral-500">Guest access · testing only</div>
          <div className="flex gap-2.5">
            <button
              type="button"
              onClick={() => onGuestLogin('developer')}
              className="btn flex-1 border border-dashed border-neutral-500 py-2.5 text-[13px] text-neutral-700 hover:bg-neutral-200 active:bg-neutral-300"
            >
              <Code size={15} strokeWidth={2.5} />
              Developer
            </button>
            <button
              type="button"
              onClick={() => onGuestLogin('alpha')}
              className="btn flex-1 border border-dashed border-neutral-500 py-2.5 text-[13px] text-neutral-700 hover:bg-neutral-200 active:bg-neutral-300"
            >
              <FlaskConical size={15} strokeWidth={2.5} />
              Alpha tester
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};

export default LoginView;
