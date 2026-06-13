import React, { useState, useRef } from 'react';
import { X, Camera, Type, Loader2, Check, Upload } from 'lucide-react';
import { analyzeFoodInput } from '../services/nutritionService';
import { FoodEntry } from '../types/nutrition';

interface AddEntryModalProps {
  isOpen: boolean;
  onClose: () => void;
  onAdd: (items: Omit<FoodEntry, 'id' | 'timestamp'>[]) => void;
}

const AddEntryModal: React.FC<AddEntryModalProps> = ({ isOpen, onClose, onAdd }) => {
  const [mode, setMode] = useState<'text' | 'image'>('text');
  const [inputText, setInputText] = useState('');
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [isAnalyzing, setIsAnalyzing] = useState(false);
  const [analysisResult, setAnalysisResult] = useState<Omit<FoodEntry, 'id' | 'timestamp'>[] | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  if (!isOpen) return null;

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files[0]) {
      const file = e.target.files[0];
      setSelectedFile(file);
      setPreviewUrl(URL.createObjectURL(file));
      setAnalysisResult(null);
    }
  };

  const handleAnalyze = async () => {
    if (mode === 'text' && !inputText.trim()) return;
    if (mode === 'image' && !selectedFile) return;

    setIsAnalyzing(true);
    setAnalysisResult(null);

    try {
      const input = mode === 'text' ? inputText : selectedFile!;
      const result = await analyzeFoodInput(input, mode);
      setAnalysisResult(result);
    } catch (err) {
      console.error(err);
      alert('Failed to analyze. Please try again.');
    } finally {
      setIsAnalyzing(false);
    }
  };

  const handleConfirm = () => {
    if (analysisResult) {
      onAdd(analysisResult);
      handleClose();
    }
  };

  const handleClose = () => {
    setInputText('');
    setSelectedFile(null);
    setPreviewUrl(null);
    setAnalysisResult(null);
    setMode('text');
    onClose();
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4 transition-opacity">
      <div className="bg-[#fdfcff] w-full max-w-lg rounded-[28px] shadow-2xl overflow-hidden flex flex-col max-h-[90vh]">
        {/* Header */}
        <div className="p-6 pb-2 flex justify-between items-center">
          <h2 className="text-2xl font-normal text-[#1d1b20]">Log Food</h2>
          <button onClick={handleClose} className="p-2 hover:bg-[#f3f0f5] rounded-full transition-colors">
            <X size={24} className="text-[#49454f]" />
          </button>
        </div>

        {/* Tabs */}
        <div className="px-6 flex gap-4 mt-4">
          <button
            onClick={() => { setMode('text'); setAnalysisResult(null); }}
            className={`flex-1 py-3 rounded-full flex items-center justify-center gap-2 transition-all ${
              mode === 'text' 
                ? 'bg-[#eaddff] text-[#21005d] font-medium' 
                : 'bg-transparent border border-[#79747e] text-[#49454f]'
            }`}
          >
            <Type size={18} />
            <span>Text</span>
          </button>
          <button
            onClick={() => { setMode('image'); setAnalysisResult(null); }}
            className={`flex-1 py-3 rounded-full flex items-center justify-center gap-2 transition-all ${
              mode === 'image' 
                ? 'bg-[#eaddff] text-[#21005d] font-medium' 
                : 'bg-transparent border border-[#79747e] text-[#49454f]'
            }`}
          >
            <Camera size={18} />
            <span>Image</span>
          </button>
        </div>

        {/* Content */}
        <div className="p-6 flex-1 overflow-y-auto">
          {!analysisResult ? (
            <div className="space-y-4">
              {mode === 'text' ? (
                <textarea
                  className="w-full h-40 bg-[#f3f0f5] rounded-xl p-4 text-[#1d1b20] focus:outline-none focus:ring-2 focus:ring-[#6750a4] resize-none placeholder-[#49454f]"
                  placeholder="e.g., A grilled chicken sandwich and a medium fries..."
                  value={inputText}
                  onChange={(e) => setInputText(e.target.value)}
                />
              ) : (
                <div 
                  className="w-full h-64 border-2 border-dashed border-[#79747e] rounded-xl flex flex-col items-center justify-center cursor-pointer hover:bg-[#f3f0f5] transition-colors relative overflow-hidden"
                  onClick={() => fileInputRef.current?.click()}
                >
                  {previewUrl ? (
                    <img src={previewUrl} alt="Preview" className="absolute inset-0 w-full h-full object-cover" />
                  ) : (
                    <>
                      <Upload size={32} className="text-[#49454f] mb-2" />
                      <p className="text-[#49454f]">Tap to upload food photo</p>
                    </>
                  )}
                  <input 
                    type="file" 
                    ref={fileInputRef} 
                    className="hidden" 
                    accept="image/*" 
                    onChange={handleFileSelect} 
                  />
                </div>
              )}

              <button
                disabled={isAnalyzing || (mode === 'text' ? !inputText : !selectedFile)}
                onClick={handleAnalyze}
                className="w-full py-4 bg-[#6750a4] text-white rounded-full font-medium shadow-md hover:shadow-lg disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2 transition-all"
              >
                {isAnalyzing ? <Loader2 className="animate-spin" /> : <React.Fragment>Analyze <span className="text-xs bg-white/20 px-2 py-0.5 rounded-full">AI</span></React.Fragment>}
              </button>
            </div>
          ) : (
            <div className="space-y-4 animate-in fade-in slide-in-from-bottom-4 duration-500">
              <h3 className="text-lg font-medium text-[#1d1b20]">Found Items</h3>
              <div className="space-y-2">
                {analysisResult.map((item, idx) => (
                  <div key={idx} className="flex justify-between items-center bg-[#f3f0f5] p-4 rounded-2xl">
                    <div>
                      <p className="font-medium text-[#1d1b20]">{item.name}</p>
                      <p className="text-sm text-[#49454f]">{item.calories} kcal</p>
                    </div>
                    <div className="flex gap-2 text-xs text-[#49454f]">
                      <span className="bg-[#e8def8] px-2 py-1 rounded-md">P: {item.protein}g</span>
                      <span className="bg-[#e8def8] px-2 py-1 rounded-md">C: {item.carbs}g</span>
                      <span className="bg-[#e8def8] px-2 py-1 rounded-md">F: {item.fat}g</span>
                    </div>
                  </div>
                ))}
              </div>
              <div className="flex gap-3 pt-4">
                 <button
                  onClick={() => setAnalysisResult(null)}
                  className="flex-1 py-3 border border-[#79747e] text-[#6750a4] rounded-full font-medium hover:bg-[#f3f0f5] transition-colors"
                >
                  Retry
                </button>
                <button
                  onClick={handleConfirm}
                  className="flex-1 py-3 bg-[#6750a4] text-white rounded-full font-medium shadow-md hover:shadow-lg flex items-center justify-center gap-2 transition-colors"
                >
                  <Check size={18} />
                  Add to Log
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default AddEntryModal;
