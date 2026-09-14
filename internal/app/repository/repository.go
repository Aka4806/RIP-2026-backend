package repository

import "errors"

const (
	Draft     = "черновик"
	Published = "опубликован"
	Deleted   = "удален"
)

type IndexConfiguration struct {
	ID          int
	Name        string
	IndexType   string
	TableSize   int
	Selectivity float64
	Description string
	Status      string
	ImageKey    string
	VideoKey    string
	Likes       []int
}

var indexConfigurations = []IndexConfiguration{
	{
		ID:          1,
		Name:        "B-tree 100K",
		IndexType:   "B-tree",
		TableSize:   100000,
		Selectivity: 0.01,
		Description: "B-tree поддерживает сравнения на равенство, диапазоны и получение данных в отсортированном порядке. " +
			"Конфигурация для таблицы из 100 000 строк.",
		Status:   Published,
		ImageKey: "btree_100k.png",
		VideoKey: "btree 100K.mp4",
		Likes:    []int{11, 24, 37, 42, 58},
	},
	{
		ID:          2,
		Name:        "Hash 100K",
		IndexType:   "Hash",
		TableSize:   100000,
		Selectivity: 0.01,
		Description: "Хеш-индекс PostgreSQL хранит хеш-код ключа и используется для сравнений на равенство. " +
			"Он не предназначен для поиска по диапазону.",
		Status:   Published,
		ImageKey: "hash_100k.png",
		VideoKey: "hash 100K.MOV",
		Likes:    []int{11, 24, 52},
	},
	{
		ID:          3,
		Name:        "Bitmap 100K",
		IndexType:   "Bitmap",
		TableSize:   100000,
		Selectivity: 0.1,
		Description: "Bitmap-индексы Oracle используют битовые карты и подходят для аналитических запросов к столбцам с небольшим числом различных значений. " +
			"Это не отдельный тип индекса PostgreSQL.",
		Status:   Published,
		ImageKey: "bitmap_100k.png",
		VideoKey: "bitmap 100K.MOV",
		Likes:    []int{24, 37, 52, 61},
	},
	{
		ID:          4,
		Name:        "B-tree 1M",
		IndexType:   "B-tree",
		TableSize:   1000000,
		Selectivity: 0.01,
		Description: "B-tree поддерживает поиск по равенству и диапазону. " +
			"Эффективность зависит от распределения данных и доли выбранных строк. " +
			"Размер таблицы — один миллион строк.",
		Status:   Published,
		ImageKey: "btree_1m.png",
		VideoKey: "btree 1M.MOV",
		Likes:    []int{11, 37, 42, 58, 61, 74},
	},
	{
		ID:          5,
		Name:        "Bitmap 1M",
		IndexType:   "Bitmap",
		TableSize:   1000000,
		Selectivity: 0.1,
		Description: "Bitmap-индексы применяются в хранилищах данных. " +
			"Частые конкурентные изменения могут сделать их неудачным выбором для транзакционной нагрузки.",
		Status:   Published,
		ImageKey: "bitmap_1m.png",
		VideoKey: "bitmap 1M.MOV",
		Likes:    []int{24, 52},
	},
	{
		ID:          6,
		Name:        "B-tree 10M",
		IndexType:   "B-tree",
		TableSize:   10000000,
		Selectivity: 0.001,
		Description: "Конфигурация B-tree для десяти миллионов строк. " +
			"Селективность здесь означает долю строк, выбранных условием; 0,001 соответствует 0,1 процента таблицы.",
		Status:   Published,
		ImageKey: "btree_10m.png",
		VideoKey: "btree 10M.MOV",
		Likes:    []int{11, 42, 74},
	},
	{
		ID:          7,
		Name:        "Hash 1M",
		IndexType:   "Hash",
		TableSize:   1000000,
		Selectivity: 0.01,
		Description: "Индексная структура для точного поиска по равенству ключа.",
		Status:      Draft,
		ImageKey:    "hash_1m.png",
		VideoKey:    "hash_1m.mp4",
		Likes:       []int{},
	},
	{
		ID:          8,
		Name:        "B-tree архив",
		IndexType:   "B-tree",
		TableSize:   10000,
		Selectivity: 0.1,
		Description: "Архивная конфигурация B-tree для таблицы из 10 000 строк.",
		Status:      Deleted,
		ImageKey:    "btree_archive.png",
		VideoKey:    "btree_archive.mp4",
		Likes:       []int{11},
	},
}

type Repository struct{}

func NewRepository() *Repository {
	return &Repository{}
}

func (r *Repository) GetIndexes(tableSize int) []IndexConfiguration {
	result := []IndexConfiguration{}
	for _, item := range indexConfigurations {
		if item.Status == Published && (tableSize == 0 || item.TableSize == tableSize) {
			result = append(result, item)
		}
	}
	return result
}

func (r *Repository) GetDraft() (IndexConfiguration, error) {
	for _, item := range indexConfigurations {
		if item.Status == Draft {
			return item, nil
		}
	}
	return IndexConfiguration{}, errors.New("черновик не найден")
}

func (r *Repository) GetIndex(id int, next bool) (IndexConfiguration, error) {
	items := r.GetIndexes(0)
	if len(items) == 0 {
		return IndexConfiguration{}, errors.New("опубликованных конфигураций нет")
	}

	position := 0
	if id != 0 {
		position = -1
		for i, item := range items {
			if item.ID == id {
				position = i
				break
			}
		}
		if position == -1 {
			return IndexConfiguration{}, errors.New("конфигурация не найдена")
		}
	}

	if next {
		position = (position + 1) % len(items)
	}
	return items[position], nil
}
