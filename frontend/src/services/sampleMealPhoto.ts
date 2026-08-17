// A meal photo shipped with the app so the AI scanner can be tried without
// one. Guests arrive from "continue as guest" to look around, and the photo
// scanner is the one feature that asks them to go find something first — a
// camera roll on a desktop browser often has nothing edible in it at all.
//
// The asset: "Dinner meal MyPlate 20210810-FNS-UNC-0016" by the USDA Food and
// Nutrition Service, public domain (a work of the U.S. federal government),
// via Wikimedia Commons — https://commons.wikimedia.org/wiki/File:Dinner_meal_MyPlate_20210810-FNS-UNC-0016.jpg
// Downscaled to 1024px and recompressed: the scanner's own resize caps photos
// at 1600px and vision models sample well below that, so a larger asset would
// only cost the guest bytes. It was picked for what it shows — rice and beans,
// mango, grilled squash, and a glass of milk are four separable items, so the
// analysis lands on the multi-item review screen rather than a single row.
import sampleMealUrl from "../assets/sample-meal.jpg";

/** Filename shown in the scanner's preview chip, so the photo reads as ours. */
const SAMPLE_FILE_NAME = "sample-meal.jpg";

export { sampleMealUrl };

export class SampleMealPhotoError extends Error {
  constructor() {
    super("The sample photo could not be loaded");
    this.name = "SampleMealPhotoError";
  }
}

/**
 * Reads the bundled sample into a File, so it enters the scanner through the
 * same path as a camera or gallery pick — validated, resized, and stripped of
 * metadata by prepareFoodPhoto like any other photo. Nothing about the
 * analysis request marks it as a sample.
 */
export async function loadSampleMealPhoto(): Promise<File> {
  let blob: Blob;
  try {
    const response = await fetch(sampleMealUrl);
    if (!response.ok) throw new SampleMealPhotoError();
    blob = await response.blob();
  } catch {
    // Offline, or the asset is missing from the deployed bundle. Either way
    // the caller has to say so rather than open an empty preview.
    throw new SampleMealPhotoError();
  }

  return new File([blob], SAMPLE_FILE_NAME, {
    type: "image/jpeg",
    lastModified: Date.now(),
  });
}
