
import axios from "axios";
import * as cheerio  from "cheerio";
import { clientPromise } from "./src/utility/database";
import "dotenv/config";


type ItemType = {
  link: string,
  title: string,
  image: string,
  brand: string,
  price: number
}

let page: number = 1;
let client: any;
let db: any;
let brands: any[] = [];

const twelveHoursAgo = new Date(Date.now() - 12 * 60 * 60 * 1000);

const connectDB = async () => {
  client = await clientPromise;
  db = await client.db(`${process.env.MONGODB_DATABASE}`);
};

const sleep = (millis: number) => {
  return new Promise((resolve) => setTimeout(resolve, millis));
};

const getRandom = (array:any[] = []) => {
  return Math.random() * (array.length - 1);
};

const savePrice = async (data: ItemType) => {
  const {title, brand, link, price} = data;
  const now = new Date();

  try{
    const ItemResponse = await db.collection("items").findOne({
      title,
      brand,
      link
    });

    const {_id} = ItemResponse
    const itemObjectId = _id.toString()

    const PricesResponse = await db.collection("prices").findOne({
      itemID: itemObjectId,
      date: { $gt: twelveHoursAgo }
    });

    if(!PricesResponse){
      const response = await db.collection("prices").insertOne({
        itemID: itemObjectId,
        date: now,
        currency: "zar",
        price,
      });

      console.log(response)
      return null;
    }

    throw new Error("Price update failed")
  } catch(error) {
    return null
  } finally {
    return null;
  }
}

const saveItem = async(data: ItemType) => {
  const {link, title, image, brand} = data;

  const filter = {
    link,
  };

  const update = {
    $set: {
      title,
      brand,
      images: [image],
      link,
      updated: new Date(),
      sources: {
        source: "amazon",
      },
    },
  };

  try{
    const options = { upsert: true };
    const cursor = await db
          .collection("items")
          .updateOne(filter, update, options);
          
    if(cursor){
      await savePrice(data)
      return null;
    }

    throw new Error("Item update failed")
  }catch(error){
    console.log(error)
    return null
  }finally{
    return null;
  }
}

const fetchItems = async (brand: string) => {
    let totalPages = 0
    try {
        const response = await axios.get(`${process.env.LINK}/s?k=${brand}&page=${page}`);
        const html = response.data;
        const $ = cheerio.load(html);

        $('.s-pagination-item').each((_idx, el) => {
          totalPages++
        });

        if(totalPages==0){
          page = 1;
          throw new Error(`${brand} does not exist on amazon`);
        }

        await $('.sg-col-4-of-12').toArray().map(async (el) => {
          try{
            const item = $(el);
            const title = item.find('span.a-size-base-plus.a-color-base.a-text-normal').text();
            const image = item.find('img.s-image').attr('src');
            const link = item.find('a.a-link-normal.a-text-normal').attr('href');
            const price = item.find('span.a-price > span.a-offscreen').text();

            const pricesArray: string[] = price.split("R")

            await saveItem({
              title,
              image,
              link: `${process.env.LINK}${link}`,
              brand,
              price: parseFloat(pricesArray[1])
            });

            return null
          }catch(error){
            return null
          }
        });
        
        console.log(`${brand}: ${page}/${totalPages}`);

        if(page >= totalPages){
          page = 1;
          throw new Error(`${brand}: items do not exist`);
        }else{
          page++
          await sleep(5000);
          await fetchItems(brand);
        }
    } catch (error) {
      console.log(error)
      await sleep(5000);
      getBrands();
    }
};

const getBrands = async () => {
  const pipeline = [
    {
      $group: {
        _id: "$brand",
      },
    },
    {
      $project: {
        _id: 0,
        brand: "$_id",
      },
    },
  ];

  try {
    await connectDB();
    brands = await db.collection("items")
      .aggregate(pipeline)
      .toArray();
    
    const random = getRandom(brands).toFixed();
    await fetchItems(brands[random].brand);

    return null;
  } catch (error) {
    await sleep(5000)
    await getBrands();
  }
};

getBrands();