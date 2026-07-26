import React, { useState, useEffect, useRef } from 'react';
import { getCompanionMessage } from '../services/nutritionService';
import { DailyGoal, NutritionData } from '../types/nutrition';

interface CompanionCharacterProps {
  stats: NutritionData;
  goals: DailyGoal;
  isLookingAtScreen?: boolean;
}

interface Particle {
  id: number;
  type: 'heart' | 'star' | 'note';
  x: number;
  y: number;
  scale: number;
}

type EyeVariant = 'open' | 'wide' | 'happy' | 'sleepy';

// The imperative handle the React tree uses to poke the cat. Every animation
// runs outside React (WAAPI + inline styles on the SVG) so idle behaviour never
// costs a re-render.
interface CatRig {
  cheer: () => void;
  setAway: (away: boolean) => void;
  destroy: () => void;
}

// How much the face slides to track the cursor / a glance, in SVG user units.
const POINTER_X = 14;
const POINTER_Y = 9;
// Playback rate for the ambient loops while the cat is dozing.
const DROWSY_RATE = 0.45;
// Settled, turned-away posture used while the user is reading content.
const AWAY_LEAN = 'translateY(6px) rotate(-5deg)';

// "Nutri" — an animated cat companion. The silhouette, the idle loops (bob,
// breathe, tail sway) and the mood repertoire are ported from the standalone
// `Cat.dc.html` design; the DCLogic lifecycle maps onto a mount effect that
// builds a `CatRig` and tears its timers down again.
//
// App integration is preserved from the previous companion: it shows a speech
// bubble fed by getCompanionMessage, reacts when the user logs food, sprinkles
// particles, and turns away when the user is busy reading content.
const CompanionCharacter: React.FC<CompanionCharacterProps> = ({ stats, goals, isLookingAtScreen = false }) => {
  const [message, setMessage] = useState<string>("Hi! I'm Nutri! (◕‿◕)");
  const [isTyping, setIsTyping] = useState(false);
  const [particles, setParticles] = useState<Particle[]>([]);

  const svgRef = useRef<SVGSVGElement>(null);
  const rigRef = useRef<CatRig | null>(null);
  // Mirrors `isLookingAtScreen` so the long-lived mood/blink loops read the
  // current value without being torn down and rebuilt on every prop change.
  const awayRef = useRef(isLookingAtScreen);
  const pendingMsgRef = useRef(false);
  const mountedRef = useRef(true);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const particleIdCounter = useRef(0);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, []);

  const addParticles = (count: number, type: Particle['type']) => {
    const newParticles: Particle[] = [];
    for (let i = 0; i < count; i++) {
      newParticles.push({
        id: particleIdCounter.current++,
        type,
        x: 20 + Math.random() * 60, // Random X pos between 20% and 80%
        y: 20 + Math.random() * 30, // Random Y pos near top of character
        scale: 0.5 + Math.random() * 0.5,
      });
    }

    setParticles(prev => [...prev, ...newParticles]);

    // Cleanup particles after animation
    setTimeout(() => {
      if (!mountedRef.current) return;
      setParticles(prev => prev.filter(p => p.id >= particleIdCounter.current - count - 10)); // Rough cleanup
    }, 2000);
  };

  // --- The cat rig ---------------------------------------------------------
  // Built once on mount. Holds every timer it schedules so unmounting (or a
  // cheer interrupting an idle mood) can cancel the whole set at once.
  useEffect(() => {
    const svg = svgRef.current;
    if (!svg) return;

    const q = <T extends SVGElement>(sel: string) => svg.querySelector<T>(sel);
    const qa = <T extends SVGElement>(sel: string) => Array.from(svg.querySelectorAll<T>(sel));

    const hop = q<SVGGElement>('[data-g="hop"]');
    const bob = q<SVGGElement>('[data-g="bob"]');
    const lean = q<SVGGElement>('[data-g="lean"]');
    const breath = q<SVGGElement>('[data-g="breath"]');
    const face = q<SVGGElement>('[data-g="face"]');
    const tail = q<SVGPathElement>('[data-g="tail"]');
    const earL = q<SVGPathElement>('[data-g="earL"]');
    const earR = q<SVGPathElement>('[data-g="earR"]');
    if (!hop || !bob || !lean || !breath || !face || !tail || !earL || !earR) return;

    const ears = { L: earL, R: earR };
    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

    const timers = new Set<ReturnType<typeof setTimeout>>();
    let pointer = { x: 0, y: 0 };
    let glance = { x: 0, y: 0 };
    let eyeState: EyeVariant = 'open';

    const after = (fn: () => void, ms: number) => {
      const t = setTimeout(() => {
        timers.delete(t);
        fn();
      }, ms);
      timers.add(t);
      return t;
    };
    const clearTimers = () => {
      timers.forEach(clearTimeout);
      timers.clear();
    };

    // Ambient loops are CSS animations; slowing them is a playback-rate nudge.
    const applyEnergy = (rate = 1) => {
      [bob, breath, tail].forEach(el => {
        el.getAnimations().forEach(a => {
          a.playbackRate = rate;
        });
      });
    };

    const setEyes = (variant: EyeVariant) => {
      const show = variant === 'happy' ? 'happy' : variant === 'sleepy' ? 'sleepy' : 'open';
      qa('[data-part]').forEach(el => {
        el.style.opacity = el.dataset.part === show ? '1' : '0';
      });
      // 'wide' is the open eye, dilated — an alert beat rather than its own shape.
      const scale = variant === 'wide' ? 1.3 : 1;
      qa('[data-part="open"]').forEach(el => {
        el.style.transform = `scale(${scale})`;
      });
      eyeState = variant;
    };

    const blink = () => {
      if (reduceMotion) return;
      if (eyeState !== 'open' && eyeState !== 'wide') return;
      const s = eyeState === 'wide' ? 1.3 : 1;
      qa('[data-part="open"]').forEach(el => {
        el.animate(
          [
            { transform: `scale(${s},${s})` },
            { transform: `scale(${s * 1.08},0.07)`, offset: 0.5 },
            { transform: `scale(${s},${s})` },
          ],
          { duration: 175, easing: 'ease-in-out' }
        );
      });
    };

    const blinkLoop = () => {
      if (reduceMotion) return;
      after(() => {
        blink();
        // Occasional double-blink keeps the rhythm from reading as metronomic.
        if (Math.random() < 0.32) after(blink, 250);
        blinkLoop();
      }, 2000 + Math.random() * 3800);
    };

    const applyFace = () => {
      face.style.transform = `translate(${pointer.x + glance.x}px,${pointer.y + glance.y}px)`;
    };

    const twitch = (side: 'L' | 'R') => {
      if (reduceMotion) return;
      const d = side === 'L' ? -14 : 14;
      ears[side].animate(
        [
          { transform: 'rotate(0deg)' },
          { transform: `rotate(${d}deg)`, offset: 0.4 },
          { transform: `rotate(${d * -0.3}deg)`, offset: 0.7 },
          { transform: 'rotate(0deg)' },
        ],
        { duration: 460, easing: 'ease-in-out' }
      );
    };

    const tailFlick = () => {
      if (reduceMotion) return;
      tail.animate(
        [
          { transform: 'rotate(0deg)' },
          { transform: 'rotate(-18deg)', offset: 0.35 },
          { transform: 'rotate(8deg)', offset: 0.7 },
          { transform: 'rotate(0deg)' },
        ],
        { duration: 620, easing: 'ease-in-out', composite: 'replace' }
      );
    };

    // A hop plus the anticipation/landing squash that sells the weight.
    const hopOnce = (height: number, dur: number) => {
      if (reduceMotion) return;
      hop.animate(
        [
          { transform: 'translateY(0)' },
          { transform: `translateY(${-height}px)`, offset: 0.42 },
          { transform: 'translateY(0)' },
        ],
        { duration: dur, easing: 'ease-in-out' }
      );
      breath.animate(
        [
          { transform: 'scale(1,1)' },
          { transform: 'scale(1.05,.9)', offset: 0.12 },
          { transform: 'scale(.96,1.06)', offset: 0.45 },
          { transform: 'scale(1.04,.93)', offset: 0.8 },
          { transform: 'scale(1,1)' },
        ],
        { duration: dur + 140, easing: 'ease-in-out' }
      );
    };

    // Resting state depends on whether the user is mid-read: a turned-away cat
    // settles back into the dozing pose instead of the alert one.
    const settle = () => {
      const away = awayRef.current;
      setEyes(away ? 'sleepy' : 'open');
      glance = away ? { x: -16, y: 4 } : { x: 0, y: 0 };
      if (away) pointer = { x: 0, y: 0 };
      applyFace();
      lean.style.transform = away ? AWAY_LEAN : 'rotate(0deg)';
      applyEnergy(away ? DROWSY_RATE : 1);
    };

    const resetPose = (ms: number) => after(settle, ms);

    // Idle personality: every few seconds pick a small beat so the cat never
    // sits perfectly still. Each beat returns to rest and schedules the next.
    const moodLoop = (delay: number): void => {
      if (reduceMotion || awayRef.current) return;
      after(() => {
        if (awayRef.current) return;
        const pick = ['look', 'look', 'twitch', 'alert', 'sleepy', 'cheerSoft', 'idle', 'idle'] as const;
        const m = pick[Math.floor(Math.random() * pick.length)];
        let hold = 2600;

        if (m === 'look') {
          const d = Math.random() < 0.5 ? -1 : 1;
          glance = { x: d * 18, y: 3 };
          applyFace();
          lean.style.transform = `rotate(${d * 2.4}deg)`;
          after(() => {
            glance = { x: -d * 12, y: 0 };
            applyFace();
            lean.style.transform = `rotate(${-d * 1.6}deg)`;
          }, 1500);
          resetPose(3000);
          hold = 3800;
        } else if (m === 'twitch') {
          twitch(Math.random() < 0.5 ? 'L' : 'R');
          hold = 1600;
        } else if (m === 'alert') {
          setEyes('wide');
          ears.L.style.transform = 'rotate(-9deg)';
          ears.R.style.transform = 'rotate(9deg)';
          tailFlick();
          after(() => {
            ears.L.style.transform = 'rotate(0deg)';
            ears.R.style.transform = 'rotate(0deg)';
          }, 900);
          resetPose(1300);
          hold = 2200;
        } else if (m === 'sleepy') {
          setEyes('sleepy');
          applyEnergy(DROWSY_RATE);
          lean.style.transform = 'translateY(8px)';
          resetPose(4200);
          hold = 5200;
        } else if (m === 'cheerSoft') {
          setEyes('happy');
          hopOnce(52, 500);
          tailFlick();
          resetPose(1400);
          hold = 2400;
        }

        moodLoop(hold + Math.random() * 1800);
      }, delay);
    };

    const cheer = () => {
      if (awayRef.current) return;
      clearTimers();
      applyEnergy();
      setEyes('happy');
      twitch('L');
      after(() => twitch('R'), 110);
      hopOnce(70, 520);
      after(() => hopOnce(46, 460), 560);
      tailFlick();
      resetPose(1500);
      blinkLoop();
      moodLoop(2600);
    };

    const onPointerMove = (e: PointerEvent) => {
      if (awayRef.current) return;
      const b = svg.getBoundingClientRect();
      if (!b.width || !b.height) return;
      const cl = (v: number, m: number) => Math.max(-m, Math.min(m, v));
      // The widget lives in a screen corner, so the cursor is usually well
      // outside it — the clamp is what turns that into "look toward the user".
      pointer = {
        x: cl(((e.clientX - b.left) / b.width - 0.5) * 2.2, 1) * POINTER_X,
        y: cl(((e.clientY - b.top) / b.height - 0.6) * 2.4, 1) * POINTER_Y,
      };
      applyFace();
    };
    window.addEventListener('pointermove', onPointerMove);

    settle();
    if (!awayRef.current) {
      blinkLoop();
      moodLoop(1200);
    }

    rigRef.current = {
      cheer,
      setAway: (away: boolean) => {
        clearTimers();
        settle();
        if (!away) {
          blinkLoop();
          moodLoop(1200);
        }
      },
      destroy: () => {
        clearTimers();
        window.removeEventListener('pointermove', onPointerMove);
      },
    };

    return () => {
      rigRef.current?.destroy();
      rigRef.current = null;
    };
  }, []);

  // Keep the rig's view of "is the user reading?" in sync, then let it re-pose.
  useEffect(() => {
    awayRef.current = isLookingAtScreen;
    rigRef.current?.setAway(isLookingAtScreen);
  }, [isLookingAtScreen]);

  // Ask the companion for a new line. Guarded so rapid pokes don't stack
  // overlapping requests.
  const requestMessage = async (kind: 'poke' | 'pull') => {
    if (isLookingAtScreen || pendingMsgRef.current) return;
    pendingMsgRef.current = true;
    setIsTyping(true);
    addParticles(kind === 'pull' ? 3 : 4, kind === 'pull' ? 'heart' : 'star');
    try {
      const msg = await getCompanionMessage(stats, goals);
      if (mountedRef.current) setMessage(msg);
    } catch {
      /* keep the previous message */
    } finally {
      pendingMsgRef.current = false;
      if (mountedRef.current) setIsTyping(false);
    }
  };

  // Petting the cat: a <button> fires this for click, Enter and Space alike.
  const handlePet = () => {
    if (isLookingAtScreen) return;
    rigRef.current?.cheer();
    requestMessage('poke');
  };

  // React when the user logs food: the calorie/protein totals jump, so Nutri
  // cheers, hums a few notes, then delivers a progress message.
  useEffect(() => {
    if (stats.calories === 0) return;

    if (timeoutRef.current) clearTimeout(timeoutRef.current);

    setIsTyping(true);
    rigRef.current?.cheer();
    const noteInterval = setInterval(() => {
      if (!isLookingAtScreen) addParticles(1, 'note');
    }, 400);

    timeoutRef.current = setTimeout(async () => {
      try {
        const msg = await getCompanionMessage(stats, goals);
        if (mountedRef.current) {
          setMessage(msg);
          addParticles(3, 'heart');
        }
      } catch {
        // keep old message
      } finally {
        if (mountedRef.current) setIsTyping(false);
        clearInterval(noteInterval);
      }
    }, 1500);

    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
      clearInterval(noteInterval);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [stats.calories, stats.protein]);

  return (
    <div className="cmp fixed bottom-24 left-4 md:bottom-8 md:left-8 z-40 flex flex-col items-center gap-2 pointer-events-none">
      <style>{CMP_STYLES}</style>

      {/* Speech Bubble */}
      <div
        className={`pointer-events-auto relative bg-surface border border-divider rounded-2xl p-3 shadow-lg max-w-[200px] mb-2 transform transition-all duration-300 origin-bottom ${
          message && !isLookingAtScreen ? 'scale-100 opacity-100' : 'scale-0 opacity-0'
        }`}
      >
        <div className="text-[13px] text-ink font-medium leading-tight">
          {isTyping ? (
            <div className="flex gap-1 items-center h-5">
              <span className="w-1.5 h-1.5 bg-accent rounded-full animate-bounce" style={{ animationDelay: '0ms' }} />
              <span className="w-1.5 h-1.5 bg-accent rounded-full animate-bounce" style={{ animationDelay: '150ms' }} />
              <span className="w-1.5 h-1.5 bg-accent rounded-full animate-bounce" style={{ animationDelay: '300ms' }} />
            </div>
          ) : message}
        </div>
        {/* Tail */}
        <div className="absolute -bottom-2 left-1/2 -translate-x-1/2 w-4 h-4 bg-surface border-b border-r border-divider transform rotate-45" />
      </div>

      {/* Character — click (or Space/Enter) to pet the cat */}
      <button
        type="button"
        aria-label="Nutri, your nutrition companion. Activate to pet the cat for a fresh message."
        className={`cmp-cat w-28 h-28 pointer-events-auto ${isLookingAtScreen ? 'is-away' : ''}`}
        onClick={handlePet}
      >
        <span className="cmp-shadow" aria-hidden="true" />

        {/* Cat silhouette. Nested groups isolate each motion channel so they
            compose without fighting: hop (WAAPI) > bob (CSS) > lean (glance
            tilt) > breathe (CSS), with the face tracking on top. */}
        <svg ref={svgRef} className="cmp-cat-svg" viewBox="392 296 792 536" aria-hidden="true" focusable="false">
          <g data-g="hop" className="cmp-hop">
            <g data-g="bob" className="cmp-bob">
              <g data-g="lean" className="cmp-lean">
                <path
                  data-g="tail"
                  className="cmp-cat-line cmp-tail"
                  d="M 1004,714 C 1074,752 1152,728 1150,648 C 1148,608 1140,588 1130,574"
                  strokeWidth="44"
                  strokeLinecap="round"
                />
                <g data-g="breath" className="cmp-breath">
                  <path
                    data-g="earL"
                    className="cmp-cat-body cmp-ear cmp-ear-l"
                    d="M 532,452 C 518,394 526,330 552,320 C 576,310 592,344 620,372 C 646,396 664,388 676,392 C 620,404 566,424 532,452 Z"
                  />
                  <path
                    data-g="earR"
                    className="cmp-cat-body cmp-ear cmp-ear-r"
                    d="M 948,452 C 962,394 954,330 928,320 C 904,310 888,344 860,372 C 834,396 816,388 804,392 C 860,404 914,424 948,452 Z"
                  />
                  <path
                    className="cmp-cat-body"
                    d="M 690,384 C 780,372 872,384 942,424 C 1014,466 1052,552 1040,646 C 1029,736 976,798 890,806 C 786,816 636,810 556,780 C 462,745 400,656 414,572 C 428,482 540,398 690,384 Z"
                  />
                </g>

                {/* Eyes are background-coloured cutouts. Each socket carries all
                    three expressions stacked; setEyes cross-fades their opacity. */}
                <g data-g="face" className="cmp-face">
                  {[612, 776].map(cx => (
                    <g key={cx} transform={`translate(${cx},566)`}>
                      <rect
                        data-part="open"
                        className="cmp-eye-fill cmp-eye-part"
                        x="-18"
                        y="-36"
                        width="36"
                        height="72"
                        rx="18"
                      />
                      <path
                        data-part="happy"
                        className="cmp-eye-line cmp-eye-part"
                        d="M -21,6 C -12,-12 12,-12 21,6"
                        strokeWidth="13"
                        strokeLinecap="round"
                        opacity="0"
                      />
                      <rect
                        data-part="sleepy"
                        className="cmp-eye-fill cmp-eye-part"
                        x="-20"
                        y="-5"
                        width="40"
                        height="10"
                        rx="5"
                        opacity="0"
                      />
                    </g>
                  ))}
                </g>
              </g>
            </g>
          </g>
        </svg>

        {/* Floating delight particles */}
        <span className="cmp-particles" aria-hidden="true">
          {particles.map((p) => (
            <span
              key={p.id}
              className="cmp-particle"
              style={{ left: `${p.x}%`, top: `${p.y}%`, transform: `scale(${p.scale})` }}
            >
              {p.type === 'heart' && (
                <svg width="20" height="20" viewBox="0 0 24 24" fill="#FFB7B2" className="drop-shadow-sm">
                  <path d="M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z" />
                </svg>
              )}
              {p.type === 'star' && (
                <svg width="24" height="24" viewBox="0 0 24 24" fill="#FFD700" className="drop-shadow-sm">
                  <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z" />
                </svg>
              )}
              {p.type === 'note' && (
                <svg width="16" height="16" viewBox="0 0 24 24" fill="#c67139" className="drop-shadow-sm">
                  <path d="M12 3v10.55c-.59-.34-1.27-.55-2-.55-2.21 0-4 1.79-4 4s1.79 4 4 4 4-1.79 4-4V7h4V3h-6z" />
                </svg>
              )}
            </span>
          ))}
        </span>
      </button>
    </div>
  );
};

// Scoped styles for the cat companion. Every selector is namespaced under
// `cmp-` so nothing leaks into the rest of the app. The body and the eye
// cutouts are painted from `--color-text` / `--color-bg`, which swap together
// in the .dark block — so the silhouette stays readable in either theme.
const CMP_STYLES = `
.cmp-cat {
  --tilt: 0deg;
  position: relative;
  display: grid;
  place-items: center;
  padding: 0;
  border: 0;
  background: transparent;
  cursor: pointer;
  transform: rotate(var(--tilt));
  transition: transform 220ms cubic-bezier(0.2, 0.8, 0.2, 1);
}
.cmp-cat:focus-visible {
  outline: 3px solid rgb(198 113 57 / 70%);
  outline-offset: 6px;
  border-radius: 22px;
}
.cmp-cat.is-away { --tilt: 4deg; cursor: default; }

.cmp-shadow {
  width: 62%;
  height: 12%;
  position: absolute;
  bottom: 12%;
  left: 50%;
  z-index: 0;
  border-radius: 50%;
  background: rgb(46 43 37 / 18%);
  filter: blur(8px);
  transform: translateX(-50%);
}

.cmp-cat-svg {
  position: relative;
  z-index: 1;
  display: block;
  width: 100%;
  height: auto;
  overflow: visible;
}

.cmp-cat-body { fill: var(--color-text); }
.cmp-cat-line { fill: none; stroke: var(--color-text); }
.cmp-eye-fill { fill: var(--color-bg); }
.cmp-eye-line { fill: none; stroke: var(--color-bg); }

/* Ambient loops. Each sits on its own group so playbackRate can slow all
   three at once when the cat dozes off. */
.cmp-hop { transform-box: fill-box; }
.cmp-bob {
  transform-box: fill-box;
  transform-origin: 50% 100%;
  animation: cmp-cat-bob 5.6s ease-in-out infinite;
}
.cmp-breath {
  transform-box: fill-box;
  transform-origin: 50% 100%;
  animation: cmp-cat-breathe 3.4s ease-in-out infinite;
}
.cmp-tail {
  transform-box: fill-box;
  transform-origin: 6% 84%;
  animation: cmp-cat-tail 4.2s ease-in-out infinite;
}

.cmp-lean {
  transform-box: fill-box;
  transform-origin: 50% 100%;
  transition: transform 600ms cubic-bezier(0.34, 1.3, 0.5, 1);
}
.cmp-face { transition: transform 350ms cubic-bezier(0.3, 1, 0.4, 1); }
.cmp-ear { transform-box: fill-box; transition: transform 400ms ease; }
.cmp-ear-l { transform-origin: 55% 100%; }
.cmp-ear-r { transform-origin: 45% 100%; }
.cmp-eye-part {
  transform-box: fill-box;
  transform-origin: 50% 50%;
  transition: opacity 160ms ease, transform 300ms cubic-bezier(0.3, 1.5, 0.4, 1);
}

.cmp-particles { position: absolute; inset: 0; pointer-events: none; overflow: visible; z-index: 4; }
.cmp-particle { position: absolute; animation: cmp-float-up 1.5s ease-out forwards; }

@keyframes cmp-cat-bob {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-9px); }
}
@keyframes cmp-cat-breathe {
  0%, 100% { transform: scale(1, 1); }
  50% { transform: scale(0.992, 1.022); }
}
@keyframes cmp-cat-tail {
  0% { transform: rotate(0deg); }
  22% { transform: rotate(-8deg); }
  58% { transform: rotate(10deg); }
  100% { transform: rotate(0deg); }
}
@keyframes cmp-float-up {
  0% { transform: translateY(0) scale(0.5); opacity: 0; }
  20% { opacity: 1; transform: translateY(-8px) scale(1); }
  100% { transform: translateY(-40px) scale(0.8); opacity: 0; }
}

/* The blink/mood/hop beats are already suppressed in JS; this stills the
   ambient CSS loops and collapses the pose transitions. */
@media (prefers-reduced-motion: reduce) {
  .cmp-bob, .cmp-breath, .cmp-tail { animation: none; }
  .cmp-lean, .cmp-face, .cmp-ear, .cmp-cat { transition-duration: 1ms; }
}
`;

export default CompanionCharacter;
