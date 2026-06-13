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

const CompanionCharacter: React.FC<CompanionCharacterProps> = ({ stats, goals, isLookingAtScreen = false }) => {
  const [message, setMessage] = useState<string>("Hi! I'm Nutri! (◕‿◕)");
  const [isTyping, setIsTyping] = useState(false);
  const [emotion, setEmotion] = useState<'happy' | 'excited' | 'sleepy' | 'eating'>('happy');
  const [isHovered, setIsHovered] = useState(false);
  const [particles, setParticles] = useState<Particle[]>([]);
  
  // Debounce ref to prevent too many API calls
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const particleIdCounter = useRef(0);

  const addParticles = (count: number, type: 'heart' | 'star' | 'note') => {
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
      setParticles(prev => prev.filter(p => p.id >= particleIdCounter.current - count - 10)); // Rough cleanup
    }, 2000);
  };

  // Fetch new message when stats change significantly (e.g., adding food)
  useEffect(() => {
    // Skip initial render if stats are empty/default to avoid wasted calls, 
    // but we allow the initial greeting to stay.
    if (stats.calories === 0) return;

    if (timeoutRef.current) clearTimeout(timeoutRef.current);

    setEmotion('eating');
    setIsTyping(true);
    // Humming notes while processing
    const noteInterval = setInterval(() => {
        if (!isLookingAtScreen) addParticles(1, 'note');
    }, 400);

    timeoutRef.current = setTimeout(async () => {
      try {
        const msg = await getCompanionMessage(stats, goals);
        setMessage(msg);
        setEmotion('happy');
        addParticles(3, 'heart'); // Happy burst on completion
      } catch (err) {
        // keep old message
      } finally {
        setIsTyping(false);
        clearInterval(noteInterval);
      }
    }, 1500); 

    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
      clearInterval(noteInterval);
    }
  }, [stats.calories, stats.protein]); 

  const handlePoke = async () => {
    if (isLookingAtScreen) return; 
    setEmotion('excited');
    addParticles(4, 'star'); // Stars when poked
    setIsTyping(true);
    try {
        const msg = await getCompanionMessage(stats, goals);
        setMessage(msg);
    } finally {
        setIsTyping(false);
        setTimeout(() => setEmotion('happy'), 2000);
    }
  };

  const handleMouseEnter = () => {
    setIsHovered(true);
    if (!isLookingAtScreen) addParticles(2, 'heart');
  };

  return (
    <div className="fixed bottom-24 right-4 md:bottom-8 md:left-24 md:right-auto z-40 flex flex-col items-center gap-2 pointer-events-auto">
      <style>{`
        @keyframes bounce-slow {
            0%, 100% { transform: translateY(0); }
            50% { transform: translateY(-10px); }
        }
        .animate-bounce-slow {
            animation: bounce-slow 3s infinite ease-in-out;
        }
        @keyframes float-up {
            0% { transform: translateY(0) scale(0.5); opacity: 0; }
            20% { opacity: 1; transform: translateY(-10px) scale(1); }
            100% { transform: translateY(-50px) scale(0.8); opacity: 0; }
        }
        .animate-particle {
            animation: float-up 1.5s ease-out forwards;
        }
      `}</style>

      {/* Speech Bubble */}
      <div 
        className={`relative bg-white border border-[#e7e0ec] rounded-2xl p-3 shadow-lg max-w-[200px] mb-2 transform transition-all duration-300 origin-bottom ${
           message && !isLookingAtScreen ? 'scale-100 opacity-100' : 'scale-0 opacity-0'
        }`}
      >
        <div className="text-sm text-[#1d1b20] font-medium leading-tight">
          {isTyping ? (
            <div className="flex gap-1 items-center h-5">
               <span className="w-1.5 h-1.5 bg-[#6750a4] rounded-full animate-bounce" style={{ animationDelay: '0ms' }}/>
               <span className="w-1.5 h-1.5 bg-[#6750a4] rounded-full animate-bounce" style={{ animationDelay: '150ms' }}/>
               <span className="w-1.5 h-1.5 bg-[#6750a4] rounded-full animate-bounce" style={{ animationDelay: '300ms' }}/>
            </div>
          ) : message}
        </div>
        {/* Tail */}
        <div className="absolute -bottom-2 left-1/2 -translate-x-1/2 w-4 h-4 bg-white border-b border-r border-[#e7e0ec] transform rotate-45"></div>
      </div>

      {/* Character Blob */}
      <div 
        className={`relative w-24 h-24 cursor-pointer transition-transform duration-500 active:scale-90 ${isLookingAtScreen ? 'translate-x-2 rotate-6' : ''}`}
        onMouseEnter={handleMouseEnter}
        onMouseLeave={() => setIsHovered(false)}
        onClick={handlePoke}
      >
        {/* Particles Container */}
        <div className="absolute inset-0 pointer-events-none overflow-visible">
            {particles.map((p) => (
                <div 
                    key={p.id}
                    className="absolute animate-particle"
                    style={{ 
                        left: `${p.x}%`, 
                        top: `${p.y}%`,
                        transform: `scale(${p.scale})` 
                    }}
                >
                    {p.type === 'heart' && (
                        <svg width="20" height="20" viewBox="0 0 24 24" fill="#FFB7B2" className="drop-shadow-sm">
                            <path d="M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z"/>
                        </svg>
                    )}
                    {p.type === 'star' && (
                         <svg width="24" height="24" viewBox="0 0 24 24" fill="#FFD700" className="drop-shadow-sm">
                            <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/>
                        </svg>
                    )}
                    {p.type === 'note' && (
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="#6750a4" className="drop-shadow-sm">
                           <path d="M12 3v10.55c-.59-.34-1.27-.55-2-.55-2.21 0-4 1.79-4 4s1.79 4 4 4 4-1.79 4-4V7h4V3h-6z"/>
                        </svg>
                    )}
                </div>
            ))}
        </div>

        {/* SVG Drawing of Commy-like character */}
        <svg viewBox="0 0 200 200" className={`w-full h-full drop-shadow-xl ${emotion === 'eating' && !isLookingAtScreen ? 'animate-pulse' : 'animate-bounce-slow'}`}>
            {/* Body - Soft White Blob */}
            <path 
                d="M100,20 C150,20 180,60 180,110 C180,160 150,190 100,190 C50,190 20,160 20,110 C20,60 50,20 100,20 Z" 
                fill="#FFFFFF"
            />
            
            {/* Front View Details */}
            {!isLookingAtScreen && (
                <>
                    {/* Cheeks */}
                    <circle cx="60" cy="115" r="12" fill="#FFB7B2" opacity="0.6" />
                    <circle cx="140" cy="115" r="12" fill="#FFB7B2" opacity="0.6" />

                    {/* Face Expressions */}
                    {emotion === 'happy' && !isHovered && (
                        <g fill="#1D1B20">
                            <circle cx="70" cy="100" r="8" />
                            <circle cx="130" cy="100" r="8" />
                            <path d="M90,120 Q100,130 110,120" stroke="#1D1B20" strokeWidth="4" fill="none" strokeLinecap="round" />
                        </g>
                    )}

                    {(emotion === 'excited' || isHovered) && (
                         <g fill="#1D1B20">
                            {/* > < eyes */}
                            <path d="M60,95 L75,105 L60,115" stroke="#1D1B20" strokeWidth="5" fill="none" strokeLinecap="round" strokeLinejoin="round"/>
                            <path d="M140,95 L125,105 L140,115" stroke="#1D1B20" strokeWidth="5" fill="none" strokeLinecap="round" strokeLinejoin="round"/>
                            <path d="M90,120 Q100,135 110,120" stroke="#1D1B20" strokeWidth="4" fill="none" strokeLinecap="round" />
                        </g>
                    )}

                    {emotion === 'eating' && (
                        <g fill="#1D1B20">
                            <circle cx="70" cy="100" r="8" />
                            <circle cx="130" cy="100" r="8" />
                            <circle cx="100" cy="130" r="10" fill="#FFB7B2" />
                        </g>
                    )}
                </>
            )}

            {/* Back/Side View Details (3/4 Turn) */}
            {isLookingAtScreen && (
                 <g>
                    {/* Tail - Shifted left to imply body turned right */}
                    <circle cx="60" cy="155" r="9" fill="#F3F0F5" />
                    
                    {/* Side Cheek - Visible on the far right edge of the profile */}
                    <circle cx="160" cy="115" r="10" fill="#FFB7B2" opacity="0.6" />
                    
                    {/* Side Eye - Curved line indicating looking forward/right */}
                    <path d="M160,95 Q170,100 160,105" stroke="#1D1B20" strokeWidth="3" fill="none" strokeLinecap="round" />
                 </g>
            )}

            {/* Little Sprout on head */}
            {/* When looking at screen (turned right), sprout should also tilt/flip */}
            <g transform={isLookingAtScreen ? "translate(15, 5) rotate(10 100 20)" : ""}>
                <path d="M100,20 Q100,0 85,5 Q100,10 100,20" fill="#81C784" />
                <path d="M100,20 Q100,-5 115,0 Q100,15 100,20" fill="#A5D6A7" />
            </g>

        </svg>

        {/* Shadow */}
        <div className="absolute -bottom-2 left-1/2 -translate-x-1/2 w-16 h-4 bg-black/10 rounded-full blur-sm" />
      </div>
    </div>
  );
};

export default CompanionCharacter;
