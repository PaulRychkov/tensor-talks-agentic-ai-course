"""Generate additional question JSON files for all key ML topics."""
import json
import os

BASE = os.path.join(os.path.dirname(__file__), "../backend/common/questions")
os.makedirs(BASE, exist_ok=True)


def make(id_, theory_id, segments, qtype, complexity, question, expected, answer, covers):
    return (id_ + ".json", {
        "id": id_,
        "theory_id": theory_id,
        "linked_segments": segments,
        "question_type": qtype,
        "complexity": complexity,
        "content": {
            "question": question,
            "expected_points": expected,
            "links_to_theory": [{"theory_id": theory_id, "segment_type": s} for s in segments],
        },
        "ideal_answer": {"text": answer, "covers": covers},
        "metadata": {"language": "ru", "created_by": "seed", "last_updated": "2026-04-10"},
    })


QUESTIONS = [
    # ── linear_regression ──────────────────────────────────────────────
    make("question_linear_regression_02", "theory_linear_regression", ["formula", "practical_notes"], "practical", 2,
         "Как интерпретируются коэффициенты линейной регрессии? Что происходит, если признаки не нормализованы?",
         ["коэффициент — изменение y на 1 единицу xi при фиксированных остальных", "без нормализации коэффициенты несопоставимы", "стандартизация делает важность признаков сравнимой"],
         "β_i: насколько изменится y при увеличении x_i на 1. Без нормализации коэффициенты отражают масштаб признаков. Стандартизация (μ=0,σ=1) делает их сопоставимыми.",
         ["интерпретация коэффициентов", "масштаб признаков", "стандартизация"]),
    make("question_linear_regression_03", "theory_linear_regression", ["definition"], "conceptual", 1,
         "Что такое линейная регрессия и какую задачу она решает?",
         ["предсказание непрерывного значения", "линейная зависимость y от признаков", "MSE как функция потерь", "y = β0 + β1x1 + ... + βnxn"],
         "Линейная регрессия предсказывает непрерывное y: y=β0+Σβi*xi. Обучение — минимизация MSE.",
         ["определение", "формула", "MSE"]),
    make("question_linear_regression_04", "theory_linear_regression", ["practical_notes"], "applied", 3,
         "Как обнаружить и устранить мультиколлинеарность в линейной регрессии?",
         ["VIF > 10 — признак мультиколлинеарности", "корреляционная матрица признаков", "решения: удалить признак / PCA / Ridge", "не влияет на предсказания, но нарушает интерпретируемость"],
         "VIF > 10 сигнализирует о мультиколлинеарности. Решения: удалить один из коррелирующих признаков, PCA, или Ridge (L2 стабилизирует коэффициенты).",
         ["VIF", "диагностика", "Ridge", "PCA"]),

    # ── logistic_regression ────────────────────────────────────────────
    make("question_logistic_regression_02", "theory_logistic_regression", ["formula", "example"], "practical", 2,
         "Как логистическая регрессия принимает решение о классификации? Какой порог по умолчанию и как его менять?",
         ["порог 0.5 по умолчанию", "вероятность через сигмоиду", "порог снижают при высокой цене FN", "ROC-кривая для выбора порога"],
         "P(y=1|x)=σ(wᵀx). По умолчанию класс 1 если P>0.5. Порог снижают для уменьшения FN (медицина). Оптимум — по ROC-кривой.",
         ["сигмоида", "порог классификации", "ROC"]),
    make("question_logistic_regression_03", "theory_logistic_regression", ["definition", "intuition"], "conceptual", 1,
         "В чём принципиальное отличие логистической регрессии от линейной?",
         ["логистическая — задача классификации", "выход — вероятность через сигмоиду", "функция потерь Log Loss, не MSE", "граница решения линейная"],
         "Линейная регрессия: непрерывное y (MSE). Логистическая: вероятность через сигмоиду, бинарная классификация, лосс — Log Loss.",
         ["задача классификации", "сигмоида", "Log Loss"]),
    make("question_logistic_regression_04", "theory_logistic_regression", ["practical_notes"], "applied", 3,
         "Как обрабатывать несбалансированные классы при обучении логистической регрессии?",
         ["class_weight=balanced", "SMOTE oversampling", "изменение порога классификации", "метрики F1, ROC-AUC вместо accuracy"],
         "class_weight=balanced автоматически взвешивает потери. SMOTE oversamples меньший класс. Снижение порога уменьшает FN. Оценка через F1, ROC-AUC.",
         ["class_weight", "SMOTE", "порог", "метрики дисбаланса"]),

    # ── gradient_descent ───────────────────────────────────────────────
    make("question_gradient_descent_02", "theory_gradient_descent", ["formula", "example"], "practical", 2,
         "В чём разница между SGD, Mini-batch GD и Batch GD? Когда применять каждый?",
         ["Batch GD: весь датасет — стабильно но медленно", "SGD: один пример — быстро но шумно", "Mini-batch: 32-512 примеров — оптимален для GPU", "практика: Mini-batch GD"],
         "Batch GD: точно, дорого. SGD: быстро, шумно. Mini-batch: векторизация GPU + достаточная точность. Стандарт: batch_size 32-256.",
         ["batch", "stochastic", "mini-batch", "практика"]),
    make("question_gradient_descent_03", "theory_gradient_descent", ["definition"], "conceptual", 1,
         "Что такое градиентный спуск? Объясните интуицию алгоритма.",
         ["итеративная оптимизация функции потерь", "движение в направлении антиградиента", "learning rate контролирует шаг", "θ = θ - α * ∇L(θ)"],
         "θ = θ - α * ∇L(θ). Двигаемся в направлении наибольшего убывания потерь. α — learning rate. Интуиция: спуск по склону маленькими шагами.",
         ["антиградиент", "learning rate", "формула"]),
    make("question_gradient_descent_04", "theory_gradient_descent", ["practical_notes"], "applied", 3,
         "Какие проблемы возникают при выборе learning rate и как их диагностировать?",
         ["большой LR: loss расходится или осциллирует", "малый LR: медленная сходимость", "LR scheduling: cosine annealing, step decay", "warmup в трансформерах"],
         "Большой LR: loss нестабилен/растёт. Малый: очень медленно. Диагностика: loss/epoch графики. Решения: cosine annealing, linear warmup, Adam.",
         ["диагностика", "scheduling", "warmup", "adaptive optimizers"]),

    # ── kmeans ─────────────────────────────────────────────────────────
    make("question_kmeans_02", "theory_kmeans", ["formula", "practical_notes"], "practical", 2,
         "Как выбрать оптимальное число кластеров k в K-means? Опишите метод локтя.",
         ["WCSS от k — ищем точку перегиба (локоть)", "silhouette score: сходство внутри кластера vs соседним", "Gap statistic как альтернатива", "обычно перебирают k=2..10"],
         "Elbow: WCSS=Σ||xi-μk||², строим от k, ищем перегиб. Silhouette=(b-a)/max(a,b), лучший k≈max(silhouette). Часто комбинируют оба.",
         ["Elbow method", "WCSS", "silhouette score"]),
    make("question_kmeans_03", "theory_kmeans", ["definition", "intuition"], "conceptual", 1,
         "Как работает алгоритм K-means? Опишите шаги итерации.",
         ["инициализация k центроидов (k-means++)", "E: назначить каждую точку ближайшему центроиду", "M: центроид = среднее точек кластера", "повторять до сходимости"],
         "EM-итерация: 1) Инициализация k-means++; 2) E-step: argmin_k ||xi-μk||²; 3) M-step: μk=mean(cluster_k); 4) До стабилизации.",
         ["инициализация", "E-step", "M-step", "сходимость"]),
    make("question_kmeans_04", "theory_kmeans", ["practical_notes"], "applied", 3,
         "Каковы ограничения K-means? В каких случаях он не подходит?",
         ["предполагает сферические кластеры одинакового размера", "чувствителен к выбросам", "нужно задавать k заранее", "не работает с нелинейными формами", "альтернативы: DBSCAN, GMM"],
         "K-means: сферические равные кластеры (евклидово). Не справляется с нелинейными формами (кольца — DBSCAN). Чувствителен к выбросам, нужно знать k. GMM для мягкой кластеризации.",
         ["ограничения", "DBSCAN", "GMM", "нелинейные формы"]),

    # ── overfitting ────────────────────────────────────────────────────
    make("question_overfitting_02", "theory_overfitting", ["practical_notes"], "practical", 2,
         "Назовите 4 основных метода борьбы с переобучением и кратко объясните механизм каждого.",
         ["L1/L2 регуляризация: штраф за большие веса", "Dropout: случайно отключает нейроны", "Early stopping: мониторинг val_loss", "Data augmentation: увеличение вариативности данных"],
         "1) L1/L2 штрафуют большие веса; 2) Dropout обнуляет p% нейронов; 3) Early stopping прекращает при росте val_loss; 4) Augmentation синтетически расширяет датасет.",
         ["регуляризация", "dropout", "early stopping", "аугментация"]),
    make("question_overfitting_03", "theory_overfitting", ["definition", "intuition"], "conceptual", 1,
         "Что такое переобучение? Как его диагностировать по кривым обучения?",
         ["хорошо на train, плохо на test", "большой gap между train и val loss", "val loss растёт пока train падает", "причина: запоминание шума"],
         "Переобучение: модель заучивает шум, теряет обобщение. Диагностика: train_loss↓, val_loss↓ затем ↑. Разрыв train/val — главный индикатор.",
         ["определение", "диагностика", "кривые обучения"]),

    # ── cross_validation ───────────────────────────────────────────────
    make("question_cross_validation_02", "theory_cross_validation", ["practical_notes"], "practical", 2,
         "Когда использовать стратифицированную кросс-валидацию и что такое Leave-One-Out CV?",
         ["стратифицированная CV сохраняет соотношение классов в каждом fold", "необходима при дисбалансе классов", "LOOCV: k=n, каждый пример — тест", "LOOCV несмещённо но вычислительно дорого"],
         "Stratified CV: каждый fold содержит ту же долю классов — критично при дисбалансе. LOOCV = k=n: минимальное смещение, O(n) обучений. Компромисс: 5-fold или 10-fold stratified.",
         ["стратификация", "дисбаланс", "LOOCV", "трейдофф"]),
    make("question_cross_validation_03", "theory_cross_validation", ["definition"], "applied", 3,
         "Как правильно использовать CV при подборе гиперпараметров, чтобы избежать data leakage?",
         ["nested CV: внешний для оценки, внутренний для гиперпараметров", "нельзя использовать один fold для обоих", "sklearn Pipeline предотвращает утечку", "финальная модель обучается на всём train"],
         "Nested CV: outer k-fold для оценки, inner CV внутри каждого fold для гиперпараметров. Один fold для обоих — data leakage. sklearn Pipeline: scaler обучается только на train-части.",
         ["nested CV", "data leakage", "Pipeline sklearn"]),

    # ── naive_bayes ────────────────────────────────────────────────────
    make("question_naive_bayes_02", "theory_naive_bayes", ["practical_notes"], "practical", 2,
         "Почему наивный байесовский классификатор называется 'наивным'? Где хорошо работает?",
         ["наивное допущение: признаки условно независимы", "в реальности обычно коррелируют", "хорошо: спам, text classification", "даже при нарушении независимости ранжирование корректно"],
         "Наивность: P(x1..xn|y)=ΠP(xi|y). Нарушается в реальности, но для text classification/spam ранжирование классов остаётся корректным.",
         ["независимость признаков", "применения", "почему работает"]),
    make("question_naive_bayes_03", "theory_naive_bayes", ["definition"], "conceptual", 1,
         "Объясните теорему Байеса применительно к классификации.",
         ["P(y|X) = P(X|y)*P(y)/P(X)", "posterior ∝ likelihood × prior", "P(X) одинаков для всех классов — игнорируем", "argmax_y P(X|y)*P(y)"],
         "P(y|X)∝P(X|y)*P(y). Posterior=Likelihood×Prior. P(X) — константа, игнорируем. Выбираем class с max posterior. Наивность: P(X|y)=ΠP(xi|y).",
         ["теорема Байеса", "posterior", "prior", "likelihood"]),

    # ── transformer ────────────────────────────────────────────────────
    make("question_transformer_02", "theory_transformer", ["formula", "example"], "practical", 3,
         "Как работает multi-head attention? Зачем несколько голов вместо одной?",
         ["каждая голова проецирует Q,K,V в подпространство dim/h", "разные головы фокусируются на разных паттернах", "выходы конкатенируются и проецируются обратно", "мощность без линейного роста параметров"],
         "h голов независимо: Attention(Qi,Ki,Vi) в dim/h подпространствах. MultiHead=Concat(head1..h)W^O. Разные головы: синтаксические, семантические, позиционные зависимости.",
         ["проекции QKV", "параллельные головы", "конкатенация", "специализация"]),
    make("question_transformer_03", "theory_transformer", ["definition"], "conceptual", 2,
         "Из каких основных компонентов состоит Transformer-блок?",
         ["Multi-Head Self-Attention: агрегация контекста", "Feed-Forward Network: нелинейное преобразование", "Layer Normalization: стабилизация", "Residual connections: предотвращение vanishing gradient"],
         "Transformer-блок: 1) MHSA + Add&Norm; 2) FFN (2 линейных + GELU/ReLU) + Add&Norm. Декодер добавляет cross-attention.",
         ["self-attention", "FFN", "LayerNorm", "residual"]),
    make("question_transformer_04", "theory_transformer", ["practical_notes"], "applied", 3,
         "Какова вычислительная сложность self-attention? Как её снижают для длинных последовательностей?",
         ["O(n²·d) по времени и памяти", "для длинных контекстов — узкое место", "Sparse attention: Longformer — локальные окна", "FlashAttention: IO-эффективная реализация"],
         "Self-attention O(n²). Решения: Sparse Attention (Longformer — локальные окна); FlashAttention — тайловое вычисление без полной QKᵀ, ускорение 2-4x; Linear Attention O(n).",
         ["O(n²)", "Longformer", "FlashAttention", "linear attention"]),

    # ── attention ──────────────────────────────────────────────────────
    make("question_attention_02", "theory_attention", ["formula"], "conceptual", 2,
         "Объясните формулу scaled dot-product attention. Зачем нужен масштабирующий коэффициент √d_k?",
         ["Attention(Q,K,V)=softmax(QKᵀ/√d_k)V", "QKᵀ — сходство запроса с ключами", "без масштабирования softmax насыщается", "√d_k нормирует дисперсию скалярного произведения"],
         "Attention(Q,K,V)=softmax(QKᵀ/√d_k)·V. Без √d_k при d_k=512 скалярные произведения растут как O(d_k), softmax насыщается — нулевые градиенты.",
         ["формула", "QKᵀ", "softmax насыщение", "√d_k"]),
    make("question_attention_03", "theory_attention", ["definition", "intuition"], "conceptual", 1,
         "Что такое механизм внимания? Объясните через аналогию Query-Key-Value.",
         ["взвешенная агрегация информации из разных позиций", "Query — что ищем, Key — индекс, Value — содержимое", "веса = softmax(Query·Key/√d)", "позволяет учитывать дальние зависимости"],
         "Attention — выборочная концентрация на релевантных частях. Аналогия: Q (запрос в БД), K (индекс), V (данные). Веса = softmax(QKᵀ/√d). Выход = взвешенная сумма V.",
         ["определение", "QKV аналогия", "веса", "дальние зависимости"]),

    # ── bert ───────────────────────────────────────────────────────────
    make("question_bert_02", "theory_bert", ["definition", "example"], "practical", 2,
         "Какие задачи предобучения используются в BERT и как они формируют двунаправленное понимание?",
         ["MLM: предсказание 15% замаскированных токенов", "NSP: следует ли предложение B за A", "MLM даёт доступ к левому И правому контексту", "NSP улучшает понимание связей между предложениями"],
         "BERT: 1) MLM — 15% токенов маскируется, модель предсказывает имея левый И правый контекст; 2) NSP — бинарная классификация пар. MLM — ключ к bidirectional representations.",
         ["MLM", "NSP", "bidirectional", "маскирование"]),
    make("question_bert_03", "theory_bert", ["practical_notes"], "applied", 3,
         "Как fine-tuning BERT отличается от обучения с нуля? Для каких задач NLP применяют?",
         ["fine-tuning: предобученные веса + classification head", "малый LR 2e-5, 2-5 эпох", "задачи: NER, sentiment, QA, NLI", "значительно лучше при малом labeled dataset"],
         "Fine-tuning: берём BERT, добавляем task-голову, LR=2e-5, 3-5 эпох. На 1-10k примерах fine-tuning≈scratch на 100k+.",
         ["fine-tuning", "LR", "задачи NLP", "vs scratch"]),

    # ── gpt ────────────────────────────────────────────────────────────
    make("question_gpt_02", "theory_gpt", ["formula", "example"], "practical", 2,
         "Как GPT генерирует текст? Что такое temperature и top-p sampling?",
         ["autoregressive: токен за токеном", "temperature>1 — случайнее, <1 — детерминированнее", "top-p: набор токенов с суммарной вероятностью p", "greedy = temperature 0"],
         "GPT: P(token|все предыдущие). Temperature T масштабирует логиты. Top-p: отбираем токены до суммы p, затем сэмплируем. Greedy=top-1.",
         ["autoregressive", "temperature", "top-p", "greedy"]),
    make("question_gpt_03", "theory_gpt", ["definition"], "conceptual", 2,
         "В чём ключевое архитектурное отличие GPT от BERT?",
         ["GPT: decoder-only, causal/unidirectional attention", "BERT: encoder-only, bidirectional", "GPT: предсказывает следующий токен; BERT: маскированные", "GPT — генерация, BERT — понимание"],
         "GPT — decoder-only, causal masking (только предыдущие токены). BERT — encoder-only, bidirectional. GPT: language modeling. BERT: MLM+NSP.",
         ["decoder vs encoder", "causal vs bidirectional", "генерация vs понимание"]),

    # ── rag ────────────────────────────────────────────────────────────
    make("question_rag_02", "theory_rag", ["formula", "example"], "practical", 2,
         "Опишите полный pipeline RAG-системы. Какова роль векторной базы данных?",
         ["indexing: документы → chunks → embeddings → vector store", "retrieval: query embedding → kNN → топ-k chunks", "generation: LLM(query + retrieved) → ответ", "vector stores: Chroma, Pinecone, Qdrant"],
         "Pipeline: 1) Indexing — chunks→embeddings→vector store; 2) Retrieval — ANN поиск топ-k; 3) Augmentation — concat query+context; 4) Generation — LLM с контекстом.",
         ["indexing", "retrieval", "ANN", "generation"]),
    make("question_rag_03", "theory_rag", ["definition"], "conceptual", 1,
         "Что такое RAG и какую проблему LLM он решает?",
         ["Retrieval-Augmented Generation", "решает knowledge cutoff", "уменьшает галлюцинации", "не требует fine-tuning при добавлении знаний"],
         "RAG дополняет LLM релевантными документами. Решает: 1) knowledge cutoff; 2) галлюцинации — ответ опирается на документы; 3) обновление без fine-tuning.",
         ["определение", "knowledge cutoff", "галлюцинации", "vs fine-tuning"]),

    # ── rlhf ──────────────────────────────────────────────────────────
    make("question_rlhf_02", "theory_rlhf", ["definition", "formula"], "practical", 3,
         "Опишите три ключевых этапа RLHF. Что такое Reward Model и как он обучается?",
         ["SFT: обучение на демонстрациях людей", "Reward Model: ранжирует ответы (Bradley-Terry)", "PPO: оптимизация LLM с KL-penalty от SFT", "RLHF = InstructGPT / ChatGPT подход"],
         "1) SFT — дообучаем LLM на примерах хороших ответов; 2) RM — предсказывает предпочтения (ранжирование пар); 3) PPO: reward=RM(ответ)-β*KL(LLM||SFT).",
         ["SFT", "Reward Model", "PPO", "KL-penalty"]),
    make("question_rlhf_03", "theory_rlhf", ["practical_notes"], "applied", 3,
         "Что такое DPO (Direct Preference Optimization) и чем он проще RLHF?",
         ["DPO исключает отдельный RM и PPO", "напрямую оптимизирует LLM на данных предпочтений", "проще в реализации, стабильнее обучения", "2 этапа: SFT reference + DPO на preference pairs"],
         "DPO переформулирует RLHF как supervised learning на парах (winner, loser). Нет RM, нет PPO. Этапы: SFT reference model + DPO contrastive loss.",
         ["DPO vs RLHF", "без RM и PPO", "preference pairs"]),

    # ── fine_tuning ────────────────────────────────────────────────────
    make("question_fine_tuning_02", "theory_fine_tuning", ["formula", "practical_notes"], "practical", 3,
         "Что такое LoRA и как она позволяет эффективно дообучать большие модели?",
         ["Low-Rank Adaptation: добавляет матрицы B и A низкого ранга", "W'=W+BA, r<<d, обучаются только B и A", "0.1-1% параметров, базовая модель заморожена", "QLoRA = LoRA + 4bit квантизация"],
         "LoRA: замораживаем W, добавляем ΔW=B·A (ранг r=4-64). При r=8, d=4096: 64k вместо 16M параметров. QLoRA + 4bit квантизация — экономия памяти в 4x.",
         ["Low-Rank", "замороженные веса", "параметры", "QLoRA"]),
    make("question_fine_tuning_03", "theory_fine_tuning", ["definition"], "conceptual", 1,
         "Что такое transfer learning и fine-tuning? В чём разница между ними?",
         ["transfer learning: использование знаний из задачи A для B", "fine-tuning: дообучение предобученной модели", "feature extraction: заморозить базу, обучить голову", "fine-tuning обновляет все или часть весов"],
         "Transfer learning — концепция. Fine-tuning — метод: дообучаем предобученную модель. Feature extraction: базу заморозили, обучаем только голову.",
         ["transfer learning", "fine-tuning", "feature extraction"]),

    # ── prompt_engineering ─────────────────────────────────────────────
    make("question_prompt_engineering_02", "theory_prompt_engineering", ["example", "practical_notes"], "practical", 2,
         "Что такое few-shot prompting и chain-of-thought? Когда каждый лучше работает?",
         ["few-shot: 2-5 примеров input→output в промпте", "CoT: рассуждение пошагово перед ответом", "few-shot: чёткий формат вывода", "CoT: многошаговые рассуждения, математика, логика"],
         "Few-shot: примеры задают формат — для классификации, NER. CoT: 'Let's think step by step' — для математики, логики. Zero-shot CoT часто не хуже few-shot CoT.",
         ["few-shot", "CoT", "zero-shot CoT", "применимость"]),
    make("question_prompt_engineering_03", "theory_prompt_engineering", ["definition"], "conceptual", 1,
         "Что такое prompt engineering? Из каких компонентов состоит эффективный промпт?",
         ["формулировка инструкций для LLM", "компоненты: роль, задача, контекст, формат, ограничения", "system prompt vs user prompt", "итеративная разработка"],
         "Prompt engineering — создание инструкций для LLM. Эффективный промпт: роль ('Ты эксперт...'), задача, контекст, формат вывода, ограничения.",
         ["определение", "компоненты промпта", "system/user"]),

    # ── tokenization ───────────────────────────────────────────────────
    make("question_tokenization_02", "theory_tokenization", ["formula", "example"], "practical", 2,
         "Объясните алгоритм BPE (Byte Pair Encoding). Как строится словарь токенизатора?",
         ["начало: символьный словарь", "на каждом шаге: самая частая пара → новый токен", "до достижения желаемого размера словаря", "используется в GPT-2, LLaMA"],
         "BPE: 1) Начальный словарь = символы; 2) Частоты пар; 3) Самая частая пара → новый токен; 4) Повторяем до размера V. Применяется в GPT-2, Llama.",
         ["алгоритм", "частоты пар", "размер словаря"]),
    make("question_tokenization_03", "theory_tokenization", ["definition"], "conceptual", 1,
         "Что такое токенизация и почему нельзя просто разбивать текст по пробелам?",
         ["разбиение текста на единицы для модели", "по пробелам: OOV для редких слов, огромный словарь", "subword: эффективный словарь + любые слова", "токен ≠ слово"],
         "Токенизация — разбиение на единицы. По пробелам: OOV, огромный словарь. Subword (BPE, WordPiece): 30k-100k токенов, любое слово → подслова.",
         ["зачем", "OOV", "subword"]),

    # ── word_embeddings ────────────────────────────────────────────────
    make("question_word_embeddings_02", "theory_word_embeddings", ["example", "practical_notes"], "practical", 2,
         "Чем Word2Vec отличается от GloVe и контекстуальных эмбеддингов (BERT)?",
         ["Word2Vec, GloVe — статические: одно слово = один вектор", "BERT — контекстуальные: зависят от окружения", "Word2Vec: CBOW/Skip-gram", "GloVe: матрица совместной встречаемости", "статические не различают многозначность"],
         "Word2Vec/GloVe: один вектор на слово. BERT: вектор зависит от контекста. Word2Vec — predictive, GloVe — count-based. Статические не работают с полисемией.",
         ["статические vs контекстуальные", "CBOW/skip-gram", "GloVe", "полисемия"]),
    make("question_word_embeddings_03", "theory_word_embeddings", ["definition"], "conceptual", 1,
         "Что такое word embeddings и почему они полезны для NLP?",
         ["плотные векторы слов в непрерывном пространстве", "семантически похожие слова — близкие векторы", "арифметика: king - man + woman = queen", "вместо one-hot — компактные представления"],
         "Word embeddings — плотные векторы (50-300 dim). Семантическая близость, арифметика смыслов. vs one-hot: 50 чисел вместо 100k нулей.",
         ["определение", "семантическая близость", "арифметика", "vs one-hot"]),

    # ── rnn ────────────────────────────────────────────────────────────
    make("question_rnn_02", "theory_rnn", ["formula"], "practical", 2,
         "Что такое vanishing gradient в RNN и почему это проблема для длинных последовательностей?",
         ["градиент затухает при BPTT", "умножение матриц: |W|<1 → экспоненциальное затухание", "нет памяти далее 10-20 шагов", "решение: LSTM/GRU, gradient clipping"],
         "BPTT умножает якобианы Π W. При |λ_max(W)|<1 градиент→0 экспоненциально. LSTM решает через cell state с аддитивными обновлениями.",
         ["BPTT", "экспоненциальное затухание", "LSTM", "gradient clipping"]),
    make("question_rnn_03", "theory_rnn", ["definition"], "conceptual", 1,
         "Что такое RNN и какую проблему решают по сравнению с обычными нейросетями?",
         ["обрабатывают последовательности переменной длины", "h_t = f(x_t, h_{t-1}) хранит информацию о прошлом", "разделение весов по времени (weight tying)", "применения: NLP, временные ряды"],
         "RNN: h_t=tanh(W_hh·h_{t-1}+W_xh·x_t). Одни веса на каждом шаге. Подходит для текста, аудио, временных рядов.",
         ["определение", "hidden state", "weight tying", "применения"]),

    # ── lstm ───────────────────────────────────────────────────────────
    make("question_lstm_02", "theory_lstm", ["formula"], "practical", 2,
         "Опишите три гейта в LSTM. Как cell state позволяет сохранять информацию на длинных дистанциях?",
         ["forget gate: что забыть из cell state", "input gate: что добавить", "output gate: что вывести из cell state", "cell state аддитивно обновляется — нет vanishing gradient"],
         "Gates (сигмоида): f_t — забыть, i_t — добавить, o_t — вывести. C_t=f_t⊙C_{t-1}+i_t⊙g_t — аддитивное обновление. Градиент течёт через ⊙f_t без умножения на W.",
         ["forget gate", "input gate", "output gate", "cell state аддитивность"]),
    make("question_lstm_03", "theory_lstm", ["definition"], "conceptual", 1,
         "В чём разница между LSTM и GRU? Когда предпочтительнее использовать GRU?",
         ["GRU: 2 гейта (reset, update) вместо 3 в LSTM", "GRU нет отдельного cell state", "GRU быстрее, на ~25% меньше параметров", "на малых датасетах GRU часто не хуже LSTM"],
         "GRU: 2 гейта (update z, reset r), нет cell state, меньше параметров. На небольших датасетах GRU≈LSTM по качеству, но быстрее. LSTM надёжнее для очень длинных последовательностей.",
         ["архитектурное отличие", "гейты", "параметры", "применимость"]),

    # ── svm ────────────────────────────────────────────────────────────
    make("question_svm_02", "theory_svm", ["formula"], "practical", 2,
         "Что такое kernel trick в SVM и зачем он нужен для нелинейно разделимых данных?",
         ["K(x,x')=φ(x)·φ(x') без явного отображения в высокое пространство", "RBF kernel: K=exp(-γ||x-x'||²) — бесконечномерное пространство", "нелинейные границы в исходном пространстве", "kernels: RBF, polynomial, sigmoid"],
         "SVM оптимизирует через скалярные произведения. Kernel trick: заменяем <xi,xj> на K(xi,xj) — произведение в новом пространстве без явного φ. RBF≡бесконечномерному φ.",
         ["kernel trick", "RBF", "нелинейная граница"]),
    make("question_svm_03", "theory_svm", ["definition"], "conceptual", 1,
         "Что такое SVM и максимальная маржа? Что такое опорные векторы?",
         ["гиперплоскость максимально разделяющая классы", "маржа = 2/||w||", "опорные векторы: точки на краях маржи", "max margin при ограничении y_i(w·x_i+b)≥1"],
         "SVM: найти w·x+b=0 с маржей 2/||w||. Опорные векторы — точки w·x+b=±1. Задача: min ||w||² при y_i(w·x_i+b)≥1.",
         ["определение", "маржа", "опорные векторы", "оптимизация"]),

    # ── random_forest ──────────────────────────────────────────────────
    make("question_random_forest_02", "theory_random_forest", ["formula", "practical_notes"], "practical", 2,
         "Как рассчитывается feature importance в Random Forest? Насколько ей можно доверять?",
         ["MDI: суммарное снижение Gini по признаку", "Permutation Importance: падение accuracy при перемешивании признака", "MDI предвзята к высококардинальным признакам", "SHAP values — более надёжная альтернатива"],
         "MDI: усредняем impurity_decrease по деревьям — предвзята к числовым признакам. Permutation: перемешиваем признак → падение accuracy на тесте. SHAP — наиболее корректны.",
         ["MDI", "permutation importance", "предвзятость", "SHAP"]),
    make("question_random_forest_03", "theory_random_forest", ["definition"], "conceptual", 1,
         "Что такое Random Forest? Какую роль играют bagging и случайные признаки?",
         ["ансамбль деревьев на bootstrap-выборках (bagging)", "случайное подмножество √p признаков в каждом узле", "prediction: majority vote / среднее", "декорреляция деревьев уменьшает variance"],
         "Random Forest: N деревьев на bootstrap-выборках. В каждом узле √p случайных признаков — декорреляция деревьев. Vote/average. Декорреляция критична для эффективности.",
         ["bagging", "bootstrap", "random subspace", "декорреляция"]),

    # ── gradient_boosting ──────────────────────────────────────────────
    make("question_gradient_boosting_02", "theory_gradient_boosting", ["formula", "practical_notes"], "practical", 3,
         "Чем XGBoost отличается от классического Gradient Boosting?",
         ["явная регуляризация: γT + 0.5λ||w||² в objective", "column/row subsampling как в RF", "weighted quantile sketch для быстрого разбиения", "cache-aware, out-of-core вычисления"],
         "XGBoost: регуляризация Ω(f)=γT+λ||w||², weighted quantile sketch (быстрее точного поиска), column/row subsampling. LightGBM: leaf-wise рост, histogram binning.",
         ["регуляризация", "quantile sketch", "subsampling", "LightGBM"]),
    make("question_gradient_boosting_03", "theory_gradient_boosting", ["definition"], "conceptual", 2,
         "Как работает Gradient Boosting? В чём отличие от Random Forest?",
         ["деревья последовательно на псевдо-остатках", "каждое дерево предсказывает отрицательный градиент", "RF параллельный (variance↓), GB последовательный (bias↓)", "GB точнее, RF менее склонен к переобучению"],
         "GB: F_m=F_{m-1}+η·h_m(псевдо-остатки). Каждое дерево исправляет ошибки предыдущего. RF — параллельный (variance). GB — последовательный (bias+variance).",
         ["последовательность", "псевдо-остатки", "GB vs RF", "bias-variance"]),

    # ── pca ────────────────────────────────────────────────────────────
    make("question_pca_02", "theory_pca", ["formula", "example"], "practical", 2,
         "Как выбрать число компонент в PCA? Что такое explained variance ratio?",
         ["explained_variance_ratio_[i] — доля дисперсии i-й компоненты", "cumsum ≥ 0.95 — стандартный порог", "scree plot: ищем локоть", "cross-validation с downstream задачей"],
         "explained_variance_ratio_[i]=λ_i/Σλ_j. Стратегии: cumsum≥0.95; scree plot — перегиб; reconstruction error на validation.",
         ["explained_variance_ratio", "cumsum", "scree plot"]),
    make("question_pca_03", "theory_pca", ["definition"], "conceptual", 1,
         "Что такое PCA? Какую геометрическую задачу он решает?",
         ["ортогональные направления максимальной дисперсии", "главные компоненты — собственные векторы ковариационной матрицы", "проекция с минимальной потерей информации", "применения: снижение размерности, визуализация"],
         "PCA: найти k ортогональных осей с максимальной дисперсией. Математически: SVD X=UΣVᵀ. Проекция X·V_k — координаты в новом пространстве.",
         ["определение", "SVD", "геометрия", "применения"]),

    # ── backpropagation ────────────────────────────────────────────────
    make("question_backpropagation_02", "theory_backpropagation", ["formula"], "practical", 3,
         "Объясните правило цепочки в контексте backpropagation. Как вычисляются градиенты?",
         ["chain rule: ∂L/∂w = ∂L/∂a · ∂a/∂z · ∂z/∂w", "backward pass: локальный градиент × upstream gradient", "δ^l = (W^{l+1})ᵀδ^{l+1} ⊙ σ'(z^l)", "autograd строит computational graph"],
         "Chain rule: ∂L/∂z=∂L/∂a·σ'(z). Backprop: δ^l=(W^{l+1})ᵀδ^{l+1}⊙σ'(z^l). ∇W^l=δ^l(a^{l-1})ᵀ. PyTorch autograd — автоматически.",
         ["chain rule", "backward pass", "delta", "autograd"]),
    make("question_backpropagation_03", "theory_backpropagation", ["definition"], "conceptual", 1,
         "Что такое обратное распространение ошибки? Как оно связано с градиентным спуском?",
         ["вычисляет градиенты функции потерь по всем весам", "применяет chain rule к вычислительному графу", "backprop != gradient descent — это алгоритм вычисления градиентов", "backprop → ∇L; optimizer → обновляет W"],
         "Backpropagation — алгоритм вычисления ∇L через chain rule. Сам не обновляет веса. Optimizer (SGD, Adam) применяет ∇L: W-=lr·∇L.",
         ["роль backprop", "chain rule", "vs gradient descent", "pipeline"]),

    # ── bias_variance ──────────────────────────────────────────────────
    make("question_bias_variance_02", "theory_bias_variance", ["formula", "example"], "practical", 2,
         "Как диагностировать high bias и high variance по кривым обучения?",
         ["high bias: и train и val error высокие, маленький gap", "high variance: train низкий, val высокий, большой gap", "high bias → усложнить модель, меньше регуляризации", "high variance → больше данных, регуляризация"],
         "High bias: оба error высоки, gap маленький — модель слишком проста. High variance: train низкий, val высокий, большой gap — переобучение.",
         ["learning curves", "high bias", "high variance", "лечение"]),
    make("question_bias_variance_03", "theory_bias_variance", ["definition"], "conceptual", 1,
         "Объясните bias-variance tradeoff. Что такое bias и variance в ML?",
         ["Bias: систематическая ошибка из-за упрощений (underfitting)", "Variance: чувствительность к конкретной выборке (overfitting)", "MSE = Bias² + Variance + Irreducible noise", "сложность: bias↓, variance↑"],
         "Bias — расхождение средних предсказаний с истиной. Variance — разброс при разных данных. E[L]=Bias²+Variance+σ². Сложная модель: bias↓, variance↑.",
         ["bias", "variance", "decomposition", "tradeoff"]),

    # ── llama ──────────────────────────────────────────────────────────
    make("question_llama_02", "theory_llama", ["definition", "formula"], "practical", 3,
         "Какие архитектурные улучшения есть в LLaMA по сравнению с оригинальным Transformer?",
         ["RoPE вместо абсолютного positional encoding", "SwiGLU активация в FFN вместо ReLU", "Pre-RMSNorm вместо LayerNorm", "GQA (Grouped Query Attention) в LLaMA 2 для ускорения"],
         "LLaMA: 1) RoPE — позиционный энкодинг через вращение в Q и K; 2) SwiGLU: FFN=SiLU(xW1)⊗xW3·W2; 3) Pre-RMSNorm; 4) GQA (несколько Q на одну KV-голову).",
         ["RoPE", "SwiGLU", "RMSNorm", "GQA"]),

    # ── vector_databases ───────────────────────────────────────────────
    make("question_vector_db_02", "theory_vector_databases", ["definition", "example"], "practical", 2,
         "Что такое HNSW и ANN? Как векторные БД находят ближайших соседей эффективно?",
         ["ANN: приближённый поиск O(log n) вместо O(n)", "HNSW: иерархический навигационный граф", "IVF: разбиение пространства на кластеры", "трейдофф: скорость vs recall"],
         "ANN vs точный kNN O(n·d). HNSW — многоуровневый граф, O(log n), recall≈0.95+. IVF — поиск в nprobe ближайших кластерах. pgvector использует IVFFlat.",
         ["ANN", "HNSW", "IVF", "recall"]),
    make("question_vector_db_03", "theory_vector_databases", ["definition"], "conceptual", 1,
         "Что такое векторные базы данных и зачем они нужны в LLM-системах?",
         ["БД для хранения и поиска dense векторов", "поиск по семантическому сходству", "ключевой компонент RAG-систем", "примеры: Chroma, Pinecone, Qdrant, pgvector"],
         "Vector DB — специализированные БД для ANN-поиска по embedding-векторам. В RAG: текст→embedding→vector DB→kNN→топ-k документов.",
         ["определение", "semantic search", "RAG", "примеры"]),

    # ── chain_of_thought ───────────────────────────────────────────────
    make("question_cot_02", "theory_chain_of_thought", ["example", "practical_notes"], "practical", 2,
         "Что такое Tree-of-Thought и чем он отличается от Chain-of-Thought?",
         ["CoT: линейная цепочка рассуждений", "ToT: дерево ветвей с оценкой и бэктрекингом", "ToT лучше для задач с несколькими путями решения", "реализуется через многократные LLM вызовы + верификатор"],
         "CoT — линейное рассуждение A→B→C. ToT — дерево: несколько ветвей, оценка (LLM-верификатор), backtracking. ToT≈BFS/DFS в пространстве рассуждений.",
         ["CoT vs ToT", "бэктрекинг", "верификатор", "применения"]),
    make("question_cot_03", "theory_chain_of_thought", ["definition"], "conceptual", 1,
         "Что такое Chain-of-Thought prompting и почему он улучшает результаты LLM?",
         ["просим LLM рассуждать пошагово перед ответом", "'Let's think step by step' — zero-shot CoT", "улучшает арифметику, логику, многошаговые задачи", "декомпозиция на посильные шаги"],
         "CoT: 'Let's think step by step' разворачивает промежуточные шаги. Декомпозиция сложной задачи на простые шаги. Эффект усиливается с размером модели.",
         ["определение", "zero-shot CoT", "почему работает", "размер модели"]),

    # ── decision_trees ─────────────────────────────────────────────────
    make("question_decision_trees_02", "theory_decision_trees", ["formula"], "practical", 2,
         "Что такое Information Gain и Gini Impurity? Как дерево выбирает лучший признак?",
         ["Gini = 1 - Σp_i²", "IG = Entropy(parent) - weighted Entropy(children)", "выбираем признак с max IG или min Gini", "CART: Gini; ID3/C4.5: Entropy"],
         "Gini=1-Σp_k². IG=H(parent)-Σ|child|/|parent|·H(child). Дерево перебирает признаки и пороги, выбирает max IG. CART использует Gini.",
         ["Gini", "Entropy", "Information Gain", "алгоритм выбора"]),
    make("question_decision_trees_03", "theory_decision_trees", ["practical_notes"], "applied", 3,
         "Как регуляризуют дерево решений? Pre-pruning и post-pruning.",
         ["pre-pruning: max_depth, min_samples_split, min_samples_leaf", "post-pruning: обрезка по валидационному множеству", "Cost Complexity Pruning (ccp_alpha) в sklearn", "без регуляризации: запоминание каждого примера"],
         "Pre-pruning: max_depth, min_samples_split — ограничиваем рост. Post-pruning: удаляем листья не улучшающие val_accuracy. CCP: min R(T)+α|T|, α через CV.",
         ["pre-pruning", "post-pruning", "CCP", "max_depth"]),
]


def main():
    created = []
    skipped = []
    for filename, content in QUESTIONS:
        path = os.path.join(BASE, filename)
        if not os.path.exists(path):
            with open(path, "w", encoding="utf-8") as f:
                json.dump(content, f, ensure_ascii=False, indent=2)
            created.append(filename)
        else:
            skipped.append(filename)

    print(f"Created: {len(created)}")
    print(f"Skipped (already exist): {len(skipped)}")
    total = len([f for f in os.listdir(BASE) if f.endswith(".json")])
    print(f"Total question files now: {total}")
    for f in created:
        print(f"  + {f}")


if __name__ == "__main__":
    main()
